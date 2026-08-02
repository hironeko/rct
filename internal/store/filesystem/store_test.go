package filesystem

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hironeko/loop-engine/internal/domain"
)

func TestCreateAndLoadCurrent(t *testing.T) {
	t.Parallel()

	project := t.TempDir()
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	run := domain.Run{
		SchemaVersion: "1.0",
		ID:            "run_test_001",
		Project:       project,
		Mode:          domain.ModeSupervised,
		Backend:       "direct",
		State:         domain.StateIntake,
		Roles: map[domain.Role]domain.RoleBinding{
			domain.RoleDesigner: {
				Role:      domain.RoleDesigner,
				Provider:  domain.ProviderClaude,
				RoleID:    "designer",
				SessionID: "loop-test-designer",
			},
		},
		RequestPath: ".loop-engine/runs/run_test_001/request.md",
		CreatedAt:   now,
		UpdatedAt:   now,
		Revision:    1,
	}

	store := New(project)
	if err := store.Create(run, "Build a loop engine"); err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	got, err := store.LoadCurrent()
	if err != nil {
		t.Fatalf("LoadCurrent() error: %v", err)
	}
	if got.ID != run.ID || got.State != run.State || got.Mode != run.Mode {
		t.Fatalf("LoadCurrent() = %#v, want run %#v", got, run)
	}

	requestPath := filepath.Join(project, ".loop-engine", "runs", run.ID, "request.md")
	request, err := os.ReadFile(requestPath)
	if err != nil {
		t.Fatalf("read request: %v", err)
	}
	if string(request) != "Build a loop engine\n" {
		t.Fatalf("request = %q", request)
	}
	requestInfo, err := os.Stat(requestPath)
	if err != nil {
		t.Fatalf("stat request: %v", err)
	}
	if requestInfo.Mode().Perm() != 0o600 {
		t.Fatalf("request mode = %o, want 600", requestInfo.Mode().Perm())
	}
}
