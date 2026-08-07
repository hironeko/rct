package controlplane

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hironeko/rct/internal/app"
	"github.com/hironeko/rct/internal/domain"
	"github.com/hironeko/rct/internal/store/filesystem"
	"github.com/hironeko/rct/web"
)

func TestServerSessionSecurityAndProgressAPI(t *testing.T) {
	server, bootstrap, client, project, run := startTestServer(t)
	defer server.Close()

	response, err := client.Get(bootstrap.URL[:len(bootstrap.URL)-4] + "/api/v1/runs/" + run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d", response.StatusCode)
	}
	if response.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("CORS header must not be set")
	}
	if !strings.Contains(response.Header.Get("Content-Security-Policy"), "default-src 'none'") {
		t.Fatal("CSP is missing")
	}
	_ = response.Body.Close()

	request, _ := http.NewRequest(http.MethodPost, server.origin+"/api/v1/session", strings.NewReader(`{"token":"wrong"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://evil.invalid")
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("wrong-origin status = %d", response.StatusCode)
	}
	_ = response.Body.Close()

	csrf := redeemSession(t, client, server)
	response, err = client.Get(server.origin + "/api/v1/session")
	if err != nil {
		t.Fatal(err)
	}
	sessionBody, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || !bytes.Contains(sessionBody, []byte(csrf)) {
		t.Fatalf("session refresh response = %d %s", response.StatusCode, sessionBody)
	}
	response, err = client.Get(server.origin + "/api/v1/runs/" + run.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("snapshot status = %d", response.StatusCode)
	}
	body, _ := io.ReadAll(response.Body)
	if bytes.Contains(body, []byte(project)) || bytes.Contains(body, []byte("private request")) {
		t.Fatalf("public API leaked private data: %s", body)
	}
	var result struct {
		Data struct {
			RunID        string `json:"run_id"`
			ProjectName  string `json:"project_name"`
			LastEventSeq uint64 `json:"last_event_seq"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatal(err)
	}
	if result.Data.RunID != run.ID || result.Data.ProjectName == "" || result.Data.LastEventSeq != 2 {
		t.Fatalf("snapshot envelope = %s", body)
	}

	response, err = client.Get(server.origin + "/api/v1/runs/" + run.ID + "/events?after_seq=1&limit=10")
	if err != nil {
		t.Fatal(err)
	}
	eventBody, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || !bytes.Contains(eventBody, []byte(`"sequence":2`)) {
		t.Fatalf("events response = %d %s", response.StatusCode, eventBody)
	}
	response, err = client.Get(server.origin + "/api/v1/runs/" + run.ID + "/events?after_seq=999")
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("invalid cursor status = %d", response.StatusCode)
	}
	_ = response.Body.Close()

	request, _ = http.NewRequest(http.MethodGet, server.origin+"/api/v1/runs", nil)
	request.Host = "evil.invalid"
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("wrong-host status = %d", response.StatusCode)
	}
	_ = response.Body.Close()
}

func TestServerRejectsRedeemTokenReuse(t *testing.T) {
	server, _, client, _, _ := startTestServer(t)
	defer server.Close()
	redeemSession(t, client, server)

	request, _ := http.NewRequest(http.MethodPost, server.origin+"/api/v1/session", strings.NewReader(`{"token":"`+server.redeemToken+`"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", server.origin)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("reused token status = %d", response.StatusCode)
	}
	_ = response.Body.Close()
}

func TestServerApprovalRequiresCSRFAndIsIdempotent(t *testing.T) {
	server, _, client, project, run := startTestServer(t)
	defer server.Close()
	store := filesystem.New(project)
	persisted, err := store.Load(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	previous := persisted.State
	persisted.State = domain.StateAwaitingApproval
	persisted.Revision++
	persisted.UpdatedAt = persisted.UpdatedAt.Add(time.Second)
	if err := store.Update(persisted, previous, "ImplementationApprovalRequested"); err != nil {
		t.Fatal(err)
	}
	calls := 0
	server.config.Approval = approvalServiceFunc(func(_ context.Context, options app.ApproveOptions) (domain.Run, error) {
		calls++
		candidate, loadErr := store.Load(options.RunID)
		if loadErr != nil {
			return domain.Run{}, loadErr
		}
		oldState := candidate.State
		candidate.State = domain.StateImplementationReady
		candidate.Revision++
		candidate.UpdatedAt = candidate.UpdatedAt.Add(time.Second)
		if updateErr := store.UpdateExpected(candidate, oldState, "HumanImplementationApprovalConsumed", options.ExpectedRevision); updateErr != nil {
			return domain.Run{}, updateErr
		}
		return candidate, nil
	})
	csrf := redeemSession(t, client, server)
	body := `{"expected_revision":` + strconv.FormatUint(persisted.Revision, 10) + `,"note":"Proceed"}`

	request, _ := http.NewRequest(http.MethodPost, server.origin+"/api/v1/runs/"+run.ID+"/approve", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://evil.invalid")
	request.Header.Set("Idempotency-Key", "approval-test-key-0001")
	request.Header.Set("X-RCT-CSRF", csrf)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusForbidden || calls != 0 {
		t.Fatalf("invalid origin response = %d, calls = %d", response.StatusCode, calls)
	}
	_ = response.Body.Close()

	request, _ = http.NewRequest(http.MethodPost, server.origin+"/api/v1/runs/"+run.ID+"/approve", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", server.origin)
	request.Header.Set("Idempotency-Key", "approval-test-key-0001")
	request.Header.Set("X-RCT-CSRF", "wrong")
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusForbidden || calls != 0 {
		t.Fatalf("invalid CSRF response = %d, calls = %d", response.StatusCode, calls)
	}
	_ = response.Body.Close()

	approve := func() *http.Response {
		t.Helper()
		retry, _ := http.NewRequest(http.MethodPost, server.origin+"/api/v1/runs/"+run.ID+"/approve", strings.NewReader(body))
		retry.Header.Set("Content-Type", "application/json")
		retry.Header.Set("Origin", server.origin)
		retry.Header.Set("Idempotency-Key", "approval-test-key-0001")
		retry.Header.Set("X-RCT-CSRF", csrf)
		result, requestErr := client.Do(retry)
		if requestErr != nil {
			t.Fatal(requestErr)
		}
		return result
	}
	for attempt := 0; attempt < 2; attempt++ {
		response = approve()
		if response.StatusCode != http.StatusOK {
			responseBody, _ := io.ReadAll(response.Body)
			t.Fatalf("approval response = %d %s", response.StatusCode, responseBody)
		}
		_ = response.Body.Close()
	}
	if calls != 1 {
		t.Fatalf("approval service calls = %d, want 1", calls)
	}
}

func TestServerServesEmbeddedSPAWithoutAPIFallback(t *testing.T) {
	server, _, client, _, run := startTestServer(t)
	defer server.Close()
	response, err := client.Get(server.origin + "/ui/runs/" + run.ID)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || !bytes.Contains(body, []byte(`<div id="root"></div>`)) {
		t.Fatalf("SPA fallback = %d %s", response.StatusCode, body)
	}
	if !strings.Contains(response.Header.Get("Content-Security-Policy"), "script-src 'self'") {
		t.Fatal("SPA response does not have the required CSP")
	}
	response, err = client.Get(server.origin + "/ui/assets/missing.js")
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("missing asset status = %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, err = client.Get(server.origin + "/api/v1/missing")
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusNotFound || !strings.HasPrefix(response.Header.Get("Content-Type"), "application/json") {
		t.Fatalf("unknown API response = %d %q", response.StatusCode, response.Header.Get("Content-Type"))
	}
	_ = response.Body.Close()
}

func TestServerSSEReplaysAfterLastEventID(t *testing.T) {
	server, _, client, _, run := startTestServer(t)
	defer server.Close()
	redeemSession(t, client, server)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.origin+"/api/v1/runs/"+run.ID+"/stream", nil)
	request.Header.Set("Last-Event-ID", "1")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("SSE response = %d %q", response.StatusCode, response.Header.Get("Content-Type"))
	}
	scanner := bufio.NewScanner(response.Body)
	foundID, foundEvent := false, false
	for scanner.Scan() {
		line := scanner.Text()
		if line == "id: 2" {
			foundID = true
		}
		if line == "event: progress" {
			foundEvent = true
		}
		if foundID && foundEvent {
			break
		}
	}
	if !foundID || !foundEvent {
		t.Fatalf("SSE replay missing: id=%v event=%v", foundID, foundEvent)
	}
}

func TestServerRejectsNonLoopbackListen(t *testing.T) {
	_, err := NewServer(Config{Listen: "0.0.0.0:0", WorkspaceRoots: []string{t.TempDir()}})
	if err == nil {
		t.Fatal("non-loopback listen address was accepted")
	}
}

func startTestServer(t *testing.T) (*Server, Bootstrap, *http.Client, string, domain.Run) {
	t.Helper()
	project := t.TempDir()
	now := time.Date(2026, 8, 7, 1, 2, 3, 0, time.UTC)
	run := domain.Run{SchemaVersion: "1.0", EventProtocolVersion: domain.ProgressSchemaVersion,
		ID: "run_20260807T010203Z_abcdef123456", Project: project, Mode: domain.ModeSupervised,
		Backend: "direct", State: domain.StatePlanReview, CreatedAt: now, UpdatedAt: now, Revision: 1,
		RequirementsPath: ".rct/runs/run_20260807T010203Z_abcdef123456/artifacts/requirements/v001.json"}
	store := filesystem.New(project)
	if err := store.Create(run, "private request"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendProgressEvent(run.ID, domain.ProgressEvent{Timestamp: now, Type: "JobStarted", Phase: "plan", JobID: "plan-r01-reviewer"}); err != nil {
		t.Fatal(err)
	}
	server, err := NewServer(Config{Listen: "127.0.0.1:0", WorkspaceRoots: []string{project}, UI: web.Dist()})
	if err != nil {
		t.Fatal(err)
	}
	bootstrap, err := server.Start()
	if err != nil {
		t.Fatal(err)
	}
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 5 * time.Second}
	return server, bootstrap, client, project, run
}

type approvalServiceFunc func(context.Context, app.ApproveOptions) (domain.Run, error)

func (function approvalServiceFunc) Approve(ctx context.Context, options app.ApproveOptions) (domain.Run, error) {
	return function(ctx, options)
}

func redeemSession(t *testing.T, client *http.Client, server *Server) string {
	t.Helper()
	request, _ := http.NewRequest(http.MethodPost, server.origin+"/api/v1/session", strings.NewReader(`{"token":"`+server.redeemToken+`"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", server.origin)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("session status = %d: %s", response.StatusCode, body)
	}
	var payload struct {
		Data struct {
			CSRFToken string `json:"csrf_token"`
		} `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	return payload.Data.CSRFToken
}
