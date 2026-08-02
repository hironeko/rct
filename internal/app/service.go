package app

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/hironeko/loop-engine/internal/domain"
	"github.com/hironeko/loop-engine/internal/providers"
	loopruntime "github.com/hironeko/loop-engine/internal/runtime"
	"github.com/hironeko/loop-engine/internal/store/filesystem"
)

type Dependencies struct {
	LookPath      func(string) (string, error)
	Getenv        func(string) string
	Now           func() time.Time
	Random        io.Reader
	Agent         providers.Gateway
	JobTimeout    time.Duration
	ProviderAuth  func(context.Context, domain.Provider) error
	ProcessRunner loopruntime.ProcessRunner
}

func DefaultDependencies() Dependencies {
	return Dependencies{
		LookPath:      exec.LookPath,
		Getenv:        os.Getenv,
		Now:           time.Now,
		Random:        rand.Reader,
		Agent:         providers.NewCLIGateway(loopruntime.DirectProcessRunner{}),
		JobTimeout:    15 * time.Minute,
		ProviderAuth:  probeProviderAuth,
		ProcessRunner: loopruntime.DirectProcessRunner{},
	}
}

type Service struct {
	deps       Dependencies
	agent      providers.Gateway
	jobTimeout time.Duration
	runner     loopruntime.ProcessRunner
}

func NewService(deps Dependencies) *Service {
	if deps.LookPath == nil {
		deps.LookPath = exec.LookPath
	}
	if deps.Getenv == nil {
		deps.Getenv = os.Getenv
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}
	if deps.Random == nil {
		deps.Random = rand.Reader
	}
	if deps.Agent == nil {
		deps.Agent = providers.NewCLIGateway(loopruntime.DirectProcessRunner{})
	}
	if deps.JobTimeout <= 0 {
		deps.JobTimeout = 15 * time.Minute
	}
	if deps.ProviderAuth == nil {
		deps.ProviderAuth = probeProviderAuth
	}
	if deps.ProcessRunner == nil {
		deps.ProcessRunner = loopruntime.DirectProcessRunner{}
	}
	return &Service{
		deps:       deps,
		agent:      deps.Agent,
		jobTimeout: deps.JobTimeout,
		runner:     deps.ProcessRunner,
	}
}

type StartOptions struct {
	Request       string
	Project       string
	Mode          string
	Backend       string
	Designer      string
	Implementer   string
	Reviewer      string
	SkipToolCheck bool
}

func (s *Service) Start(_ context.Context, options StartOptions) (domain.Run, error) {
	request := strings.TrimSpace(options.Request)
	if request == "" {
		return domain.Run{}, errors.New("request is required")
	}

	project, err := filepath.Abs(options.Project)
	if err != nil {
		return domain.Run{}, fmt.Errorf("resolve project path: %w", err)
	}
	info, err := os.Stat(project)
	if err != nil {
		return domain.Run{}, fmt.Errorf("inspect project path: %w", err)
	}
	if !info.IsDir() {
		return domain.Run{}, fmt.Errorf("project path is not a directory: %s", project)
	}

	mode, err := domain.ParseRunMode(options.Mode)
	if err != nil {
		return domain.Run{}, err
	}
	roles, err := domain.ResolveRoles(domain.RoleInputs{
		Designer:    options.Designer,
		Implementer: options.Implementer,
		Reviewer:    options.Reviewer,
	})
	if err != nil {
		return domain.Run{}, err
	}
	if !options.SkipToolCheck {
		for _, provider := range roles.Providers() {
			if _, lookupErr := s.deps.LookPath(provider.Executable()); lookupErr != nil {
				return domain.Run{}, fmt.Errorf(
					"provider %q is required but executable %q was not found: %w",
					provider,
					provider.Executable(),
					lookupErr,
				)
			}
		}
	}

	requestedBackend, err := loopruntime.ParseBackend(options.Backend)
	if err != nil {
		return domain.Run{}, err
	}
	probe := s.probeRuntime()
	backend, err := loopruntime.Select(requestedBackend, probe)
	if err != nil {
		return domain.Run{}, err
	}

	runID, err := s.newRunID()
	if err != nil {
		return domain.Run{}, fmt.Errorf("create run id: %w", err)
	}
	shortID := runID
	if len(shortID) > 8 {
		shortID = shortID[len(shortID)-8:]
	}
	now := s.deps.Now().UTC()
	run := domain.Run{
		SchemaVersion: "1.0",
		ID:            runID,
		Project:       project,
		Mode:          mode,
		Backend:       string(backend),
		State:         domain.StateIntake,
		Roles: map[domain.Role]domain.RoleBinding{
			domain.RoleDesigner: {
				Role:      domain.RoleDesigner,
				Provider:  roles.Designer,
				RoleID:    string(domain.RoleDesigner),
				SessionID: "loop-" + shortID + "-designer",
			},
			domain.RoleImplementer: {
				Role:      domain.RoleImplementer,
				Provider:  roles.Implementer,
				RoleID:    string(domain.RoleImplementer),
				SessionID: "loop-" + shortID + "-implementer",
			},
			domain.RoleReviewer: {
				Role:      domain.RoleReviewer,
				Provider:  roles.Reviewer,
				RoleID:    string(domain.RoleReviewer),
				SessionID: "loop-" + shortID + "-reviewer",
			},
		},
		RequestPath: filepath.ToSlash(
			filepath.Join(".loop-engine", "runs", runID, "request.md"),
		),
		MaxReviewRounds: 3,
		CreatedAt:       now,
		UpdatedAt:       now,
		Revision:        1,
	}

	store := filesystem.New(project)
	if err := store.Create(run, request); err != nil {
		return domain.Run{}, err
	}
	return run, nil
}

type DiagnosticCheck struct {
	Name     string `json:"name"`
	OK       bool   `json:"ok"`
	Required bool   `json:"required"`
	Detail   string `json:"detail"`
}

type DoctorOptions struct {
	Project     string
	Backend     string
	Designer    string
	Implementer string
	Reviewer    string
}

type DoctorReport struct {
	Healthy bool                  `json:"healthy"`
	Backend string                `json:"backend,omitempty"`
	Roles   *domain.ResolvedRoles `json:"roles,omitempty"`
	Runtime loopruntime.Probe     `json:"runtime"`
	Checks  []DiagnosticCheck     `json:"checks"`
}

func (s *Service) Doctor(ctx context.Context, options DoctorOptions) DoctorReport {
	report := DoctorReport{
		Healthy: true,
		Runtime: s.probeRuntime(),
	}
	add := func(name string, ok bool, required bool, detail string) {
		report.Checks = append(report.Checks, DiagnosticCheck{
			Name:     name,
			OK:       ok,
			Required: required,
			Detail:   detail,
		})
		if required && !ok {
			report.Healthy = false
		}
	}

	project, err := filepath.Abs(options.Project)
	if err == nil {
		var info os.FileInfo
		info, err = os.Stat(project)
		if err == nil && !info.IsDir() {
			err = errors.New("not a directory")
		}
	}
	add("project", err == nil, true, detail(err, project))

	roles, roleErr := domain.ResolveRoles(domain.RoleInputs{
		Designer:    options.Designer,
		Implementer: options.Implementer,
		Reviewer:    options.Reviewer,
	})
	add("role_assignment", roleErr == nil, true, detail(roleErr, "role assignment is valid"))
	if roleErr == nil {
		report.Roles = &roles
		for _, provider := range roles.Providers() {
			_, lookupErr := s.deps.LookPath(provider.Executable())
			add(
				"provider_"+string(provider),
				lookupErr == nil,
				true,
				detail(lookupErr, provider.Executable()+" executable found"),
			)
			if lookupErr == nil {
				authErr := s.deps.ProviderAuth(ctx, provider)
				add(
					"provider_"+string(provider)+"_auth",
					authErr == nil,
					true,
					detail(authErr, provider.Executable()+" authentication is available"),
				)
			}
		}
	}

	requested, backendParseErr := loopruntime.ParseBackend(options.Backend)
	if backendParseErr != nil {
		add("backend", false, true, backendParseErr.Error())
	} else {
		selected, selectErr := loopruntime.Select(requested, report.Runtime)
		add("backend", selectErr == nil, true, detail(selectErr, string(selected)))
		if selectErr == nil {
			report.Backend = string(selected)
		}
	}
	add("herdr", report.Runtime.HerdrReady(), false, runtimeDetail("herdr", report.Runtime.HerdrReady()))
	add("tmux", report.Runtime.TmuxBinary, false, runtimeDetail("tmux", report.Runtime.TmuxBinary))
	return report
}

func probeProviderAuth(ctx context.Context, provider domain.Provider) error {
	probeContext, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var args []string
	switch provider {
	case domain.ProviderCodex:
		args = []string{"login", "status"}
	case domain.ProviderClaude:
		args = []string{"auth", "status"}
	default:
		return fmt.Errorf("unsupported provider %q", provider)
	}
	command := exec.CommandContext(probeContext, provider.Executable(), args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("%s is not authenticated", provider)
	}
	return nil
}

func (s *Service) probeRuntime() loopruntime.Probe {
	_, herdrErr := s.deps.LookPath("herdr")
	_, tmuxErr := s.deps.LookPath("tmux")
	managed := s.deps.Getenv("HERDR_ENV") == "1" ||
		strings.TrimSpace(s.deps.Getenv("HERDR_SOCKET_PATH")) != ""
	return loopruntime.Probe{
		HerdrBinary:  herdrErr == nil,
		HerdrManaged: managed,
		TmuxBinary:   tmuxErr == nil,
	}
}

func (s *Service) newRunID() (string, error) {
	random := make([]byte, 6)
	if _, err := io.ReadFull(s.deps.Random, random); err != nil {
		return "", err
	}
	return "run_" + s.deps.Now().UTC().Format("20060102T150405Z") + "_" + hex.EncodeToString(random), nil
}

func detail(err error, success string) string {
	if err != nil {
		return err.Error()
	}
	return success
}

func runtimeDetail(name string, available bool) string {
	if available {
		return name + " is available"
	}
	return name + " is not available; this is optional"
}
