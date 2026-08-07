package filesystem

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hironeko/rct/internal/domain"
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
				SessionID: "rct-test-designer",
			},
		},
		RequestPath: ".rct/runs/run_test_001/request.md",
		CreatedAt:   now,
		UpdatedAt:   now,
		Revision:    1,
	}

	store := New(project)
	if err := store.Create(run, "Build rct"); err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	got, err := store.LoadCurrent()
	if err != nil {
		t.Fatalf("LoadCurrent() error: %v", err)
	}
	if got.ID != run.ID || got.State != run.State || got.Mode != run.Mode {
		t.Fatalf("LoadCurrent() = %#v, want run %#v", got, run)
	}

	requestPath := filepath.Join(project, ".rct", "runs", run.ID, "request.md")
	request, err := os.ReadFile(requestPath)
	if err != nil {
		t.Fatalf("read request: %v", err)
	}
	if string(request) != "Build rct\n" {
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

func TestActivityAndProgressEvents(t *testing.T) {
	project := t.TempDir()
	now := time.Date(2026, 8, 6, 1, 2, 3, 0, time.UTC)
	run := domain.Run{SchemaVersion: "1.0", EventProtocolVersion: domain.ProgressSchemaVersion,
		ID: "run_test_progress", Project: project, Mode: domain.ModeSupervised, Backend: "direct",
		State: domain.StatePlanReview, RequirementsPath: "requirements", ArchitecturePath: "architecture",
		CreatedAt: now, UpdatedAt: now, Revision: 1}
	store := New(project)
	if err := store.Create(run, "Build rct"); err != nil {
		t.Fatal(err)
	}

	activity, err := store.WriteActivity(domain.CurrentActivity{
		RunID: run.ID, Status: domain.ActivityRunning, Phase: "plan", Action: "reviewing",
		Backend: "direct", JobID: "plan-r02-reviewer", StartedAt: now, LastHeartbeatAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if activity.Revision != 1 {
		t.Fatalf("revision = %d, want 1", activity.Revision)
	}
	activity, err = store.WriteActivity(activity)
	if err != nil {
		t.Fatal(err)
	}
	if activity.Revision != 2 {
		t.Fatalf("revision = %d, want 2", activity.Revision)
	}

	event, err := store.AppendProgressEvent(run.ID, domain.ProgressEvent{Timestamp: now, Type: "JobStarted", JobID: activity.JobID})
	if err != nil {
		t.Fatal(err)
	}
	if event.Sequence != 2 {
		t.Fatalf("sequence = %d, want 2", event.Sequence)
	}
	snapshot, err := store.Progress(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.LastEventSeq != 2 || snapshot.Activity == nil || snapshot.Activity.JobID != activity.JobID {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	info, err := os.Stat(filepath.Join(store.RunDir(run.ID), "activity.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("activity mode = %o", info.Mode().Perm())
	}
}

func TestLegacyEventReaderUsesPhysicalLineSequence(t *testing.T) {
	project := t.TempDir()
	now := time.Date(2026, 8, 6, 1, 2, 3, 0, time.UTC)
	run := domain.Run{SchemaVersion: "1.0", ID: "run_legacy", Project: project,
		Mode: domain.ModeSupervised, Backend: "direct", State: domain.StateIntake,
		CreatedAt: now, UpdatedAt: now, Revision: 1}
	store := New(project)
	if err := store.Create(run, "legacy"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendProgressEvent(run.ID, domain.ProgressEvent{Timestamp: now, Type: "JobStarted"}); err != nil {
		t.Fatal(err)
	}
	events, err := store.ReadEvents(run.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Sequence != 2 {
		t.Fatalf("events = %#v", events)
	}
	if _, err := store.Load("../escape"); err == nil || errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Load traversal error = %v", err)
	}
}

func TestProjectWriterLeaseIsExclusiveAndRecoverable(t *testing.T) {
	t.Parallel()

	store := New(t.TempDir())
	first, err := store.AcquireProjectWriterLease()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AcquireProjectWriterLease(); !errors.Is(err, ErrProjectWriterBusy) {
		t.Fatalf("second lease error = %v, want %v", err, ErrProjectWriterBusy)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	third, err := store.AcquireProjectWriterLease()
	if err != nil {
		t.Fatalf("lease after release: %v", err)
	}
	if err := third.Close(); err != nil {
		t.Fatal(err)
	}
}
