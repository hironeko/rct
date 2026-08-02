package app

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hironeko/rct/internal/domain"
)

func TestStartDesignerClaudeDerivesIndependentRoles(t *testing.T) {
	t.Parallel()

	project := t.TempDir()
	service := NewService(Dependencies{
		LookPath: func(name string) (string, error) {
			switch name {
			case "codex", "claude":
				return "/usr/local/bin/" + name, nil
			default:
				return "", errors.New("not found")
			}
		},
		Getenv: func(string) string { return "" },
		Now: func() time.Time {
			return time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
		},
		Random: bytes.NewReader([]byte{1, 2, 3, 4, 5, 6}),
	})

	run, err := service.Start(context.Background(), StartOptions{
		Request:  "Build rct",
		Project:  project,
		Mode:     "supervised",
		Backend:  "auto",
		Designer: "claude",
	})
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	if got := run.Roles[domain.RoleDesigner].Provider; got != domain.ProviderClaude {
		t.Fatalf("designer = %q, want claude", got)
	}
	if got := run.Roles[domain.RoleImplementer].Provider; got != domain.ProviderClaude {
		t.Fatalf("implementer = %q, want claude", got)
	}
	if got := run.Roles[domain.RoleReviewer].Provider; got != domain.ProviderCodex {
		t.Fatalf("reviewer = %q, want codex", got)
	}

	sessionIDs := map[string]bool{}
	for _, role := range []domain.Role{
		domain.RoleDesigner,
		domain.RoleImplementer,
		domain.RoleReviewer,
	} {
		sessionID := run.Roles[role].SessionID
		if sessionIDs[sessionID] {
			t.Fatalf("duplicate session id %q", sessionID)
		}
		sessionIDs[sessionID] = true
	}

	statePath := filepath.Join(project, ".rct", "runs", run.ID, "state.json")
	if _, err := os.Stat(statePath); err != nil {
		t.Fatalf("state file was not created: %v", err)
	}
}

func TestStartRejectsSelfReview(t *testing.T) {
	t.Parallel()

	service := NewService(Dependencies{
		LookPath: func(name string) (string, error) {
			return "/usr/local/bin/" + name, nil
		},
		Getenv: func(string) string { return "" },
		Now:    time.Now,
		Random: bytes.NewReader([]byte{1, 2, 3, 4, 5, 6}),
	})

	_, err := service.Start(context.Background(), StartOptions{
		Request:  "Build rct",
		Project:  t.TempDir(),
		Mode:     "supervised",
		Backend:  "direct",
		Designer: "claude",
		Reviewer: "claude",
	})
	if !errors.Is(err, domain.ErrRoleAssignmentConflict) {
		t.Fatalf("Start() error = %v, want role assignment conflict", err)
	}
}

func TestDoctorReportsProviderAuthenticationFailure(t *testing.T) {
	t.Parallel()

	service := NewService(Dependencies{
		LookPath: func(name string) (string, error) {
			return "/usr/local/bin/" + name, nil
		},
		Getenv: func(string) string { return "" },
		ProviderAuth: func(_ context.Context, provider domain.Provider) error {
			if provider == domain.ProviderClaude {
				return errors.New("not authenticated")
			}
			return nil
		},
	})
	report := service.Doctor(context.Background(), DoctorOptions{
		Project: t.TempDir(),
		Backend: "direct",
	})
	if report.Healthy {
		t.Fatal("Doctor() healthy = true, want false")
	}
	found := false
	for _, check := range report.Checks {
		if check.Name == "provider_claude_auth" && !check.OK && check.Required {
			found = true
		}
	}
	if !found {
		t.Fatalf("Doctor() checks = %#v, want required Claude auth failure", report.Checks)
	}
}
