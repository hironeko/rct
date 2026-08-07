package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/hironeko/rct/internal/domain"
	"github.com/hironeko/rct/internal/store/filesystem"
)

func TestExecutePlanningApprovesArchitectureAndPlan(t *testing.T) {
	t.Parallel()

	project := t.TempDir()
	gateway := &scriptedGateway{}
	service := NewService(Dependencies{
		LookPath: func(name string) (string, error) { return "/usr/local/bin/" + name, nil },
		Getenv:   func(string) string { return "" },
		Now: func() time.Time {
			return time.Date(2026, 8, 2, 13, 0, 0, 0, time.UTC)
		},
		Random:        bytes.NewReader([]byte{1, 2, 3, 4, 5, 6}),
		Agent:         gateway,
		ProviderAuth:  func(context.Context, domain.Provider) error { return nil },
		ProcessRunner: &implementationRunner{},
	})
	run, err := service.Start(context.Background(), StartOptions{
		Request: "Build it",
		Project: project,
		Mode:    "supervised",
		Backend: "direct",
	})
	if err != nil {
		t.Fatal(err)
	}
	requirements := testRequirements(t, "Approved")
	storePath, err := filesystem.New(project).WriteRunFile(
		run.ID,
		"artifacts/requirements/v001.json",
		requirements,
	)
	if err != nil {
		t.Fatal(err)
	}
	run.RequirementsPath = storePath
	run.State = domain.StateRequirementsApproved

	architecture := testArchitecture(t)
	plan := testPlan(t)
	architecturePath := documentPath(run.ID, "architecture", 1)
	planPath := documentPath(run.ID, "plan", 1)
	gateway.outputs = [][]byte{
		architecture,
		testReviewForType(
			t,
			run.ID,
			"architecture-r01-reviewer",
			"architecture",
			architecturePath,
			sha256Hex(withTrailingNewline(architecture)),
			"application/json",
			domain.VerdictApproved,
			false,
		),
		plan,
		testReviewForType(
			t,
			run.ID,
			"plan-r01-reviewer",
			"plan",
			planPath,
			sha256Hex(withTrailingNewline(plan)),
			"application/json",
			domain.VerdictApproved,
			false,
		),
	}

	run, err = service.ExecutePlanning(context.Background(), run, 3)
	if err != nil {
		t.Fatalf("ExecutePlanning() error: %v", err)
	}
	if run.State != domain.StateAwaitingApproval {
		t.Fatalf("state = %q, want %q", run.State, domain.StateAwaitingApproval)
	}
	if run.ArchitectureRound != 1 || run.PlanRound != 1 {
		t.Fatalf("rounds = architecture:%d plan:%d", run.ArchitectureRound, run.PlanRound)
	}
	if run.ApprovalTargetHash != sha256Hex(withTrailingNewline(plan)) {
		t.Fatalf("approval target = %q", run.ApprovalTargetHash)
	}
	if len(gateway.jobs) != 4 {
		t.Fatalf("jobs = %d, want 4", len(gateway.jobs))
	}
}

func testArchitecture(t *testing.T) []byte {
	t.Helper()
	data, err := json.Marshal(map[string]any{
		"schema_version": "1.0",
		"title":          "Architecture",
		"summary":        "Summary",
		"decisions": []any{map[string]any{
			"id": "ADR-001", "title": "Boundary", "decision": "Use ports",
			"rationale": "Testability", "consequences": []string{"More types"},
		}},
		"components": []any{map[string]any{
			"name": "Core", "responsibilities": []string{"Orchestrate"},
			"interfaces": []string{"Service"},
		}},
		"data_flows":         []string{"Request to artifact"},
		"quality_attributes": []string{"Safe"},
		"risks":              []string{},
		"requirement_traceability": []any{map[string]any{
			"requirement_id": "REQ-001", "decision_ids": []string{"ADR-001"},
		}},
		"open_questions": []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func testPlan(t *testing.T) []byte {
	t.Helper()
	data, err := json.Marshal(map[string]any{
		"schema_version": "1.0",
		"title":          "Plan",
		"summary":        "Summary",
		"milestones": []any{map[string]any{
			"id": "M01", "objective": "Implement", "scope": []string{"Core"},
			"non_scope": []string{}, "dependencies": []string{},
			"change_areas":        []string{"internal"},
			"acceptance_criteria": []string{"Tests pass"},
			"verification_commands": []any{map[string]any{
				"executable": "go", "args": []string{"test", "./..."},
			}},
			"risks": []string{}, "done_when": []string{"Reviewed"},
		}},
		"requirement_traceability": []any{map[string]any{
			"requirement_id": "REQ-001", "milestone_ids": []string{"M01"},
		}},
		"risks":          []string{},
		"open_questions": []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func documentPath(runID, kind string, round int) string {
	return filepath.ToSlash(filepath.Join(
		".rct", "runs", runID, "artifacts", kind,
		fmt.Sprintf("v%03d.json", round),
	))
}
