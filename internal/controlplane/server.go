package controlplane

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hironeko/rct/internal/app"
	"github.com/hironeko/rct/internal/domain"
)

const sessionCookieName = "rct_session"

type Config struct {
	Listen         string
	WorkspaceRoots []string
	UI             fs.FS
}

type Bootstrap struct {
	URL          string `json:"url"`
	BootstrapURL string `json:"-"`
}

type Server struct {
	config      Config
	catalog     *Catalog
	query       app.ProgressQueryService
	redeemToken string
	sessionID   string
	csrfToken   string
	redeemMu    sync.Mutex
	redeemed    bool
	listener    net.Listener
	httpServer  *http.Server
	origin      string
	done        chan error
}

type envelope struct {
	Data      any          `json:"data,omitempty"`
	Error     *publicError `json:"error,omitempty"`
	RequestID string       `json:"request_id"`
}

type publicError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func NewServer(config Config) (*Server, error) {
	if strings.TrimSpace(config.Listen) == "" {
		config.Listen = "127.0.0.1:0"
	}
	if err := validateLoopbackListen(config.Listen); err != nil {
		return nil, err
	}
	catalog, err := NewCatalog(config.WorkspaceRoots)
	if err != nil {
		return nil, err
	}
	redeem, err := randomHex(32)
	if err != nil {
		return nil, err
	}
	session, err := randomHex(32)
	if err != nil {
		return nil, err
	}
	csrf, err := randomHex(32)
	if err != nil {
		return nil, err
	}
	return &Server{config: config, catalog: catalog, redeemToken: redeem, sessionID: session, csrfToken: csrf, done: make(chan error, 1)}, nil
}

func (s *Server) Start() (Bootstrap, error) {
	if s.listener != nil {
		return Bootstrap{}, errors.New("control plane server is already started")
	}
	listener, err := net.Listen("tcp", s.config.Listen)
	if err != nil {
		return Bootstrap{}, fmt.Errorf("listen for control plane: %w", err)
	}
	s.listener = listener
	s.origin = "http://" + listener.Addr().String()
	s.httpServer = &http.Server{
		Handler:           s.routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 * 1024,
	}
	go func() {
		err := s.httpServer.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		s.done <- err
	}()
	return Bootstrap{URL: s.origin + "/ui/", BootstrapURL: s.origin + "/ui/#bootstrap=" + s.redeemToken}, nil
}

func (s *Server) Wait(ctx context.Context) error {
	select {
	case err := <-s.done:
		return err
	case <-ctx.Done():
		shutdownContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if s.httpServer != nil {
			if err := s.httpServer.Shutdown(shutdownContext); err != nil {
				return err
			}
		}
		return <-s.done
	}
}

func (s *Server) Close() error {
	if s.httpServer == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/session", s.handleSession)
	mux.HandleFunc("GET /api/v1/runs", s.requireSession(s.handleRuns))
	mux.HandleFunc("HEAD /api/v1/runs", s.requireSession(s.handleRuns))
	mux.HandleFunc("GET /api/v1/runs/", s.requireSession(s.handleRunRoute))
	mux.HandleFunc("HEAD /api/v1/runs/", s.requireSession(s.handleRunRoute))
	mux.HandleFunc("GET /ui/", s.handleUI)
	mux.HandleFunc("HEAD /ui/", s.handleUI)
	mux.HandleFunc("GET /api/", func(writer http.ResponseWriter, _ *http.Request) {
		s.writeError(writer, http.StatusNotFound, "not_found", "The API endpoint was not found")
	})
	mux.HandleFunc("GET /", func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, "/ui/", http.StatusTemporaryRedirect)
	})
	return s.securityHeaders(s.enforceHost(mux))
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; font-src 'none'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'")
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		writer.Header().Set("X-Frame-Options", "DENY")
		writer.Header().Set("Referrer-Policy", "no-referrer")
		writer.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(writer, request)
	})
}

func (s *Server) enforceHost(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Host != s.listener.Addr().String() {
			s.writeError(writer, http.StatusForbidden, "forbidden_origin", "The request host is not allowed")
			return
		}
		next.ServeHTTP(writer, request)
	})
}

func (s *Server) requireSession(next http.HandlerFunc) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		cookie, err := request.Cookie(sessionCookieName)
		if err != nil || !constantEqual(cookie.Value, s.sessionID) {
			s.writeError(writer, http.StatusUnauthorized, "unauthorized", "A local rct session is required")
			return
		}
		next(writer, request)
	}
}

func (s *Server) handleSession(writer http.ResponseWriter, request *http.Request) {
	if request.Header.Get("Origin") != s.origin {
		s.writeError(writer, http.StatusForbidden, "forbidden_origin", "The request origin is not allowed")
		return
	}
	if contentType := request.Header.Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
		s.writeError(writer, http.StatusUnsupportedMediaType, "invalid_input", "JSON content is required")
		return
	}
	var input struct {
		Token string `json:"token"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		s.writeError(writer, http.StatusBadRequest, "invalid_input", "The session bootstrap request is invalid")
		return
	}
	s.redeemMu.Lock()
	valid := !s.redeemed && constantEqual(input.Token, s.redeemToken)
	if valid {
		s.redeemed = true
	}
	s.redeemMu.Unlock()
	if !valid {
		s.writeError(writer, http.StatusUnauthorized, "unauthorized", "The bootstrap token is invalid or has already been used")
		return
	}
	http.SetCookie(writer, &http.Cookie{
		Name: sessionCookieName, Value: s.sessionID, Path: "/", HttpOnly: true,
		SameSite: http.SameSiteStrictMode, MaxAge: 12 * 60 * 60,
	})
	s.writeJSON(writer, http.StatusOK, map[string]any{"csrf_token": s.csrfToken})
}

func (s *Server) handleRuns(writer http.ResponseWriter, request *http.Request) {
	runs, err := s.catalog.List()
	if err != nil {
		s.writeError(writer, http.StatusInternalServerError, "internal_error", "The run catalog could not be read")
		return
	}
	results := make([]app.PublicRunSnapshot, 0, len(runs))
	for _, located := range runs {
		snapshot, err := s.query.Snapshot(located.Project, located.Run.ID)
		if err == nil {
			results = append(results, snapshot)
		}
	}
	s.writeJSON(writer, http.StatusOK, results)
}

func (s *Server) handleRunRoute(writer http.ResponseWriter, request *http.Request) {
	relative := strings.TrimPrefix(request.URL.Path, "/api/v1/runs/")
	parts := strings.Split(strings.Trim(relative, "/"), "/")
	if len(parts) == 0 || parts[0] == "" || len(parts) > 2 {
		s.writeError(writer, http.StatusNotFound, "not_found", "The run endpoint was not found")
		return
	}
	located, err := s.catalog.Resolve(parts[0])
	if err != nil {
		s.writeError(writer, http.StatusNotFound, "not_found", "The run was not found")
		return
	}
	if len(parts) == 1 {
		snapshot, err := s.query.Snapshot(located.Project, located.Run.ID)
		if err != nil {
			s.writeError(writer, http.StatusInternalServerError, "internal_error", "The run snapshot could not be read")
			return
		}
		s.writeJSON(writer, http.StatusOK, snapshot)
		return
	}
	switch parts[1] {
	case "activity":
		activity, err := s.query.Activity(located.Project, located.Run.ID)
		if err != nil {
			s.writeError(writer, http.StatusInternalServerError, "internal_error", "The run activity could not be read")
			return
		}
		s.writeJSON(writer, http.StatusOK, activity)
	case "events":
		s.handleEvents(writer, request, located)
	case "stream":
		s.handleStream(writer, request, located)
	default:
		s.writeError(writer, http.StatusNotFound, "not_found", "The run endpoint was not found")
	}
}

func (s *Server) handleEvents(writer http.ResponseWriter, request *http.Request, located LocatedRun) {
	after, err := parseUintQuery(request.URL.Query(), "after_seq", 0)
	if err != nil {
		s.writeError(writer, http.StatusBadRequest, "invalid_input", "after_seq must be an unsigned integer")
		return
	}
	limitValue, err := parseUintQuery(request.URL.Query(), "limit", 100)
	if err != nil || limitValue == 0 || limitValue > 256 {
		s.writeError(writer, http.StatusBadRequest, "invalid_input", "limit must be between 1 and 256")
		return
	}
	snapshot, err := s.query.Snapshot(located.Project, located.Run.ID)
	if err != nil {
		s.writeError(writer, http.StatusInternalServerError, "internal_error", "The run snapshot could not be read")
		return
	}
	if err := app.ValidateEventCursor(after, snapshot.LastEventSeq); err != nil {
		s.writeError(writer, http.StatusConflict, "resync_required", "The event cursor must be resynchronized from the current snapshot")
		return
	}
	events, next, err := s.query.Events(located.Project, located.Run.ID, after, int(limitValue))
	if err != nil {
		s.writeError(writer, http.StatusConflict, "resync_required", "The event stream must be resynchronized from the current snapshot")
		return
	}
	s.writeJSON(writer, http.StatusOK, map[string]any{"events": events, "next_after_seq": next})
}

func (s *Server) handleStream(writer http.ResponseWriter, request *http.Request, located LocatedRun) {
	flusher, ok := writer.(http.Flusher)
	if !ok {
		s.writeError(writer, http.StatusInternalServerError, "internal_error", "Streaming is unavailable")
		return
	}
	after, err := parseLastEventID(request)
	if err != nil {
		s.writeError(writer, http.StatusBadRequest, "invalid_input", "Last-Event-ID must be an unsigned integer")
		return
	}
	snapshot, err := s.query.Snapshot(located.Project, located.Run.ID)
	if err != nil {
		s.writeError(writer, http.StatusInternalServerError, "internal_error", "The run snapshot could not be read")
		return
	}
	if err := app.ValidateEventCursor(after, snapshot.LastEventSeq); err != nil {
		s.writeError(writer, http.StatusConflict, "resync_required", "The event cursor must be resynchronized from the current snapshot")
		return
	}
	writer.Header().Set("Content-Type", "text/event-stream")
	writer.Header().Set("Connection", "keep-alive")
	writer.Header().Set("X-Accel-Buffering", "no")
	writer.WriteHeader(http.StatusOK)
	flusher.Flush()

	cursor := after
	activityRevision := uint64(0)
	if snapshot.Activity != nil {
		activityRevision = snapshot.Activity.Revision
	}
	poll := time.NewTicker(500 * time.Millisecond)
	keepalive := time.NewTicker(15 * time.Second)
	defer poll.Stop()
	defer keepalive.Stop()
	for {
		if !s.replayEvents(writer, flusher, located, &cursor) {
			return
		}
		select {
		case <-request.Context().Done():
			return
		case <-keepalive.C:
			if !writeSSE(writer, flusher, "", "", []byte(": keepalive\n\n")) {
				return
			}
		case <-poll.C:
			current, err := s.query.Snapshot(located.Project, located.Run.ID)
			if err != nil {
				return
			}
			currentRevision := uint64(0)
			if current.Activity != nil {
				currentRevision = current.Activity.Revision
			}
			if currentRevision != activityRevision {
				activityRevision = currentRevision
				data, _ := json.Marshal(current.Activity)
				if !writeSSE(writer, flusher, "", "activity", data) {
					return
				}
			}
			if terminalRunState(current.State) && cursor >= current.LastEventSeq {
				data, _ := json.Marshal(current)
				_ = writeSSE(writer, flusher, "", "terminal", data)
				return
			}
		}
	}
}

func (s *Server) replayEvents(writer http.ResponseWriter, flusher http.Flusher, located LocatedRun, cursor *uint64) bool {
	for {
		events, next, err := s.query.Events(located.Project, located.Run.ID, *cursor, 256)
		if err != nil {
			data, _ := json.Marshal(map[string]string{"code": "resync_required"})
			_ = writeSSE(writer, flusher, "", "resync_required", data)
			return false
		}
		for _, event := range events {
			data, _ := json.Marshal(event)
			if !writeSSE(writer, flusher, strconv.FormatUint(event.Sequence, 10), "progress", data) {
				return false
			}
		}
		*cursor = next
		if len(events) < 256 {
			return true
		}
	}
}

func writeSSE(writer http.ResponseWriter, flusher http.Flusher, id, event string, data []byte) bool {
	controller := http.NewResponseController(writer)
	_ = controller.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if len(data) > 0 && data[0] == ':' {
		_, err := writer.Write(data)
		if err == nil {
			flusher.Flush()
		}
		return err == nil
	}
	if id != "" {
		if _, err := fmt.Fprintf(writer, "id: %s\n", id); err != nil {
			return false
		}
	}
	if event != "" {
		if _, err := fmt.Fprintf(writer, "event: %s\n", event); err != nil {
			return false
		}
	}
	if _, err := fmt.Fprintf(writer, "data: %s\n\n", data); err != nil {
		return false
	}
	flusher.Flush()
	return true
}

func (s *Server) handleUI(writer http.ResponseWriter, request *http.Request) {
	if s.config.UI == nil {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		writer.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(writer, "<!doctype html><html lang=\"en\"><head><meta charset=\"utf-8\"><title>rct</title></head><body><main><h1>rct</h1><p>The browser UI is not included in this build.</p></main></body></html>")
		return
	}
	clean := strings.TrimPrefix(path.Clean(request.URL.Path), "/ui/")
	if clean == "." || clean == "" {
		clean = "index.html"
	}
	if strings.HasPrefix(clean, "assets/") {
		s.serveUIFile(writer, request, clean, false)
		return
	}
	if _, err := fs.Stat(s.config.UI, clean); err == nil {
		s.serveUIFile(writer, request, clean, false)
		return
	}
	s.serveUIFile(writer, request, "index.html", true)
}

func (s *Server) serveUIFile(writer http.ResponseWriter, request *http.Request, name string, fallback bool) {
	data, err := fs.ReadFile(s.config.UI, name)
	if err != nil {
		if fallback {
			s.writeError(writer, http.StatusServiceUnavailable, "ui_unavailable", "The browser UI is unavailable")
		} else {
			s.writeError(writer, http.StatusNotFound, "not_found", "The UI asset was not found")
		}
		return
	}
	switch {
	case strings.HasSuffix(name, ".html"):
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	case strings.HasSuffix(name, ".js"):
		writer.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	case strings.HasSuffix(name, ".css"):
		writer.Header().Set("Content-Type", "text/css; charset=utf-8")
	case strings.HasSuffix(name, ".svg"):
		writer.Header().Set("Content-Type", "image/svg+xml")
	}
	writer.Header().Set("Content-Length", strconv.Itoa(len(data)))
	writer.WriteHeader(http.StatusOK)
	if request.Method != http.MethodHead {
		_, _ = writer.Write(data)
	}
}

func (s *Server) writeJSON(writer http.ResponseWriter, status int, data any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(envelope{Data: data, RequestID: requestID()})
}

func (s *Server) writeError(writer http.ResponseWriter, status int, code, message string) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(envelope{Error: &publicError{Code: code, Message: message}, RequestID: requestID()})
}

func validateLoopbackListen(address string) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("invalid listen address: %w", err)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() || host != "127.0.0.1" {
		return errors.New("control plane listen address must use 127.0.0.1")
	}
	return nil
}

func randomHex(bytes int) (string, error) {
	data := make([]byte, bytes)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return hex.EncodeToString(data), nil
}

func requestID() string {
	value, err := randomHex(8)
	if err != nil {
		return "req_unavailable"
	}
	return "req_" + value
}

func constantEqual(left, right string) bool {
	if len(left) != len(right) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
}

func parseUintQuery(values url.Values, name string, fallback uint64) (uint64, error) {
	value := values.Get(name)
	if value == "" {
		return fallback, nil
	}
	return strconv.ParseUint(value, 10, 64)
}

func parseLastEventID(request *http.Request) (uint64, error) {
	value := request.Header.Get("Last-Event-ID")
	if value == "" {
		value = request.URL.Query().Get("after_seq")
	}
	if value == "" {
		return 0, nil
	}
	return strconv.ParseUint(value, 10, 64)
}

func terminalRunState(state domain.WorkflowState) bool {
	switch state {
	case domain.StateCompleted, domain.StateFailed, domain.StateBlocked, domain.StateWaitingForHuman, domain.StateAwaitingApproval:
		return true
	default:
		return false
	}
}
