package app

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/hironeko/rct/internal/domain"
	"github.com/hironeko/rct/internal/store/filesystem"
)

func TestProgressQueryRedactsPrivateRunData(t *testing.T) {
	project := t.TempDir()
	now := time.Date(2026, 8, 7, 1, 2, 3, 0, time.UTC)
	run := domain.Run{
		SchemaVersion: "1.0", EventProtocolVersion: domain.ProgressSchemaVersion,
		ID: "run_20260807T010203Z_abcdef123456", Project: project, Mode: domain.ModeSupervised,
		Backend: "direct", State: domain.StateFailed, CreatedAt: now, UpdatedAt: now, Revision: 1,
		RequirementsPath: ".rct/runs/run_20260807T010203Z_abcdef123456/artifacts/requirements/v001.json",
		Failure:          "secret-token=do-not-return",
		Roles: map[domain.Role]domain.RoleBinding{
			domain.RoleDesigner: {Role: domain.RoleDesigner, Provider: domain.ProviderCodex},
			domain.RoleReviewer: {Role: domain.RoleReviewer, Provider: domain.ProviderClaude},
		},
	}
	store := filesystem.New(project)
	if err := store.Create(run, "private prompt body"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.WriteActivity(domain.CurrentActivity{
		RunID: run.ID, Status: domain.ActivityFailed, Phase: "plan", Action: "reviewing",
		Provider: "claude", Backend: "direct", JobID: "plan-r01-reviewer", StartedAt: now,
		LastHeartbeatAt: now, WaitingReason: "secret waiting reason",
		Error: &domain.SafeProgressError{Code: "UNKNOWN", Summary: "secret stderr", NextAction: project},
	}); err != nil {
		t.Fatal(err)
	}

	snapshot, err := (ProgressQueryService{}).Snapshot(project, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	for _, private := range []string{project, "private prompt body", "secret-token", "secret waiting reason", "secret stderr"} {
		if strings.Contains(text, private) {
			t.Fatalf("public snapshot contains private value %q: %s", private, text)
		}
	}
	if snapshot.Artifacts["requirements"] != "artifacts/requirements/v001.json" {
		t.Fatalf("artifact reference = %q", snapshot.Artifacts["requirements"])
	}
	if snapshot.Activity == nil || snapshot.Activity.Error == nil || snapshot.Activity.Error.Code != "RUN_FAILED" {
		t.Fatalf("safe activity error = %#v", snapshot.Activity)
	}
}

func TestProgressQueryEventsOmitsSummaryAndData(t *testing.T) {
	project := t.TempDir()
	now := time.Date(2026, 8, 7, 1, 2, 3, 0, time.UTC)
	run := domain.Run{SchemaVersion: "1.0", EventProtocolVersion: domain.ProgressSchemaVersion,
		ID: "run_20260807T010203Z_abcdef123456", Project: project, Mode: domain.ModeSupervised,
		Backend: "direct", State: domain.StateIntake, CreatedAt: now, UpdatedAt: now, Revision: 1}
	store := filesystem.New(project)
	if err := store.Create(run, "request"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendProgressEvent(run.ID, domain.ProgressEvent{
		Timestamp: now, Type: "JobStarted", JobID: "plan-r01-reviewer",
		Summary: "secret summary", Data: map[string]any{"token": "secret"},
	}); err != nil {
		t.Fatal(err)
	}
	events, next, err := (ProgressQueryService{}).Events(project, run.ID, 1, 100)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(events)
	if strings.Contains(string(encoded), "secret") {
		t.Fatalf("public events contain private data: %s", encoded)
	}
	if len(events) != 1 || next != 2 || events[0].JobID != "plan-r01-reviewer" {
		t.Fatalf("events = %#v, next=%d", events, next)
	}
}
