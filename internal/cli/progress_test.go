package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/hironeko/rct/internal/app"
	"github.com/hironeko/rct/internal/domain"
	"github.com/hironeko/rct/internal/store/filesystem"
)

func TestSnapshotLineShowsMacroGaugeAndCurrentJob(t *testing.T) {
	now := time.Now().UTC()
	snapshot := domain.ProgressSnapshot{
		State:  domain.StatePlanReview,
		Gauges: []domain.Gauge{{Completed: 2, Total: 8, Label: "phases complete"}},
		Activity: &domain.CurrentActivity{Status: domain.ActivityRunning, Phase: "plan", Action: "reviewing",
			Provider: "claude", JobID: "plan-r02-reviewer", Round: 2, MaxRounds: 3, LastHeartbeatAt: now},
	}
	line := snapshotLine(snapshot)
	for _, expected := range []string{"2/8 phases complete", "Plan", "Claude reviewing", "round 2/3"} {
		if !strings.Contains(line, expected) {
			t.Fatalf("line %q does not contain %q", line, expected)
		}
	}
}

func TestPlainEventUsesStableMachineReadableShape(t *testing.T) {
	var output bytes.Buffer
	renderPlainEvent(&output, domain.ProgressEvent{Timestamp: time.Date(2026, 8, 6, 1, 2, 3, 0, time.UTC),
		Type: "JobStarted", Phase: "plan", Role: "reviewer", Provider: "claude", Round: 2, JobID: "plan-r02-reviewer"})
	got := output.String()
	for _, expected := range []string{"job_started", "phase=plan", "role=reviewer", "provider=claude", "round=2", "job=plan-r02-reviewer"} {
		if !strings.Contains(got, expected) {
			t.Fatalf("output %q does not contain %q", got, expected)
		}
	}
}

func TestNotificationPayloadIsFixedAndRunIDValidated(t *testing.T) {
	title, body, ok := notificationText(domain.ProgressEvent{Type: "RunFailed", Summary: "secret raw output"})
	if !ok || title != "rct: run failed" || body != "Open rct status for the next action" {
		t.Fatalf("notification = %q %q %v", title, body, ok)
	}
	if safeShortRunID("../../unsafe") != "" {
		t.Fatal("unsafe run id was accepted")
	}
	if got := safeShortRunID("run_20260806T010203Z_abcdef123456"); got != "abcdef12" {
		t.Fatalf("short id = %q", got)
	}
}

func TestStatusAndWatchUseProgressSnapshot(t *testing.T) {
	project := t.TempDir()
	now := time.Date(2026, 8, 6, 1, 2, 3, 0, time.UTC)
	run := domain.Run{SchemaVersion: "1.0", EventProtocolVersion: domain.ProgressSchemaVersion,
		ID: "run_20260806T010203Z_abcdef123456", Project: project, Mode: domain.ModeSupervised,
		Backend: "direct", State: domain.StatePlanReview, RequirementsPath: "requirements",
		ArchitecturePath: "architecture", CreatedAt: now, UpdatedAt: now, Revision: 1}
	store := filesystem.New(project)
	if err := store.Create(run, "request"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.WriteActivity(domain.CurrentActivity{RunID: run.ID, Status: domain.ActivityRunning,
		Phase: "plan", Action: "reviewing", Provider: "claude", Backend: "direct", JobID: "plan-r02-reviewer",
		Round: 2, MaxRounds: 3, StartedAt: now, LastHeartbeatAt: now}); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	command := New(app.NewService(app.DefaultDependencies()), strings.NewReader(""), &stdout, &stderr)
	if code := command.Run(context.Background(), []string{"status", "--project", project}); code != 0 {
		t.Fatalf("status code=%d stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Overall progress: 2/8") || !strings.Contains(stdout.String(), "plan-r02-reviewer") {
		t.Fatalf("status output = %q", stdout.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := command.Run(context.Background(), []string{"watch", "--project", project, "--format", "jsonl"}); code != 0 {
		t.Fatalf("watch code=%d stderr=%q", code, stderr.String())
	}
	var envelope map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("watch JSON = %q: %v", stdout.String(), err)
	}
	if envelope["kind"] != "snapshot" {
		t.Fatalf("watch envelope = %#v", envelope)
	}
}
