package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hironeko/loop-engine/internal/domain"
	"github.com/hironeko/loop-engine/internal/providers"
)

type scriptedGateway struct {
	outputs [][]byte
	jobs    []providers.Job
}

func (g *scriptedGateway) Execute(
	_ context.Context,
	job providers.Job,
) (providers.Result, error) {
	g.jobs = append(g.jobs, job)
	if len(g.outputs) == 0 {
		return providers.Result{}, errors.New("no scripted output")
	}
	output := g.outputs[0]
	g.outputs = g.outputs[1:]
	return providers.Result{StructuredOutput: output}, nil
}

func TestExecuteDesignRevisesThenApproves(t *testing.T) {
	t.Parallel()

	project := t.TempDir()
	gateway := &scriptedGateway{}
	service := NewService(Dependencies{
		LookPath: func(name string) (string, error) {
			return "/usr/local/bin/" + name, nil
		},
		Getenv: func(string) string { return "" },
		Now: func() time.Time {
			return time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
		},
		Random:       bytes.NewReader([]byte{1, 2, 3, 4, 5, 6}),
		Agent:        gateway,
		ProviderAuth: func(context.Context, domain.Provider) error { return nil },
	})
	run, err := service.Start(context.Background(), StartOptions{
		Request:  "Build a loop engine",
		Project:  project,
		Mode:     "design-only",
		Backend:  "direct",
		Designer: "codex",
	})
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	first := testRequirements(t, "First")
	revised := testRequirements(t, "Revised")
	gateway.outputs = [][]byte{
		first,
		testReview(
			t,
			run.ID,
			"requirements-r01-reviewer",
			requirementsPath(run.ID, 1),
			sha256Hex(withTrailingNewline(first)),
			domain.VerdictChangesRequested,
			true,
		),
		revised,
		testReview(
			t,
			run.ID,
			"requirements-r02-reviewer",
			requirementsPath(run.ID, 2),
			sha256Hex(withTrailingNewline(revised)),
			domain.VerdictApproved,
			false,
		),
	}

	run, err = service.ExecuteDesign(context.Background(), run, 3)
	if err != nil {
		t.Fatalf("ExecuteDesign() error: %v", err)
	}
	if run.State != domain.StateRequirementsApproved {
		t.Fatalf("state = %q, want %q", run.State, domain.StateRequirementsApproved)
	}
	if run.RequirementsRound != 2 {
		t.Fatalf("round = %d, want 2", run.RequirementsRound)
	}
	if run.LastVerdict != domain.VerdictApproved {
		t.Fatalf("verdict = %q, want approved", run.LastVerdict)
	}
	if len(gateway.jobs) != 4 {
		t.Fatalf("jobs = %d, want 4", len(gateway.jobs))
	}
	if gateway.jobs[0].Provider != domain.ProviderCodex ||
		gateway.jobs[1].Provider != domain.ProviderClaude {
		t.Fatalf("provider order = %s, %s", gateway.jobs[0].Provider, gateway.jobs[1].Provider)
	}
	if !bytes.Contains(gateway.jobs[2].Prompt, []byte("Required review feedback")) {
		t.Fatal("revision prompt does not include review feedback")
	}
	if _, err := os.Stat(filepath.Join(
		project,
		".loop-engine",
		"runs",
		run.ID,
		"artifacts",
		"requirements",
		"v002.json",
	)); err != nil {
		t.Fatalf("revised artifact not stored: %v", err)
	}
}

func TestExecuteDesignStopsAtReviewLimit(t *testing.T) {
	t.Parallel()

	gateway := &scriptedGateway{}
	service := NewService(Dependencies{
		LookPath: func(name string) (string, error) {
			return "/usr/local/bin/" + name, nil
		},
		Getenv:       func(string) string { return "" },
		Now:          time.Now,
		Random:       bytes.NewReader([]byte{1, 2, 3, 4, 5, 6}),
		Agent:        gateway,
		ProviderAuth: func(context.Context, domain.Provider) error { return nil },
	})
	run, err := service.Start(context.Background(), StartOptions{
		Request: "Build it",
		Project: t.TempDir(),
		Mode:    "design-only",
		Backend: "direct",
	})
	if err != nil {
		t.Fatal(err)
	}
	first := testRequirements(t, "First")
	gateway.outputs = [][]byte{
		first,
		testReview(
			t,
			run.ID,
			"requirements-r01-reviewer",
			requirementsPath(run.ID, 1),
			sha256Hex(withTrailingNewline(first)),
			domain.VerdictChangesRequested,
			true,
		),
	}
	run, err = service.ExecuteDesign(context.Background(), run, 1)
	if err != nil {
		t.Fatalf("ExecuteDesign() error: %v", err)
	}
	if run.State != domain.StateWaitingForHuman {
		t.Fatalf("state = %q, want WAITING_FOR_HUMAN", run.State)
	}
}

func TestExecuteDesignRejectsReviewForStaleArtifact(t *testing.T) {
	t.Parallel()

	gateway := &scriptedGateway{}
	service := NewService(Dependencies{
		LookPath: func(name string) (string, error) {
			return "/usr/local/bin/" + name, nil
		},
		Getenv:       func(string) string { return "" },
		Now:          time.Now,
		Random:       bytes.NewReader([]byte{1, 2, 3, 4, 5, 6}),
		Agent:        gateway,
		ProviderAuth: func(context.Context, domain.Provider) error { return nil },
	})
	run, err := service.Start(context.Background(), StartOptions{
		Request: "Build it",
		Project: t.TempDir(),
		Mode:    "design-only",
		Backend: "direct",
	})
	if err != nil {
		t.Fatal(err)
	}
	artifact := testRequirements(t, "First")
	gateway.outputs = [][]byte{
		artifact,
		testReview(
			t,
			run.ID,
			"requirements-r01-reviewer",
			requirementsPath(run.ID, 1),
			strings.Repeat("0", 64),
			domain.VerdictApproved,
			false,
		),
	}

	run, err = service.ExecuteDesign(context.Background(), run, 1)
	if err == nil || !strings.Contains(err.Error(), "does not match current artifact") {
		t.Fatalf("ExecuteDesign() error = %v, want stale artifact rejection", err)
	}
	if run.State != domain.StateFailed {
		t.Fatalf("state = %q, want FAILED", run.State)
	}
}

func testRequirements(t *testing.T, title string) []byte {
	t.Helper()
	data, err := json.Marshal(map[string]any{
		"schema_version":    "1.0",
		"title":             title,
		"summary":           "Summary",
		"problem_statement": "Problem",
		"goals":             []string{"Goal"},
		"non_goals":         []string{},
		"assumptions":       []string{},
		"constraints":       []string{},
		"requirements": []any{
			map[string]any{
				"id":                  "REQ-001",
				"title":               "Requirement",
				"statement":           "Do the thing",
				"priority":            "must",
				"acceptance_criteria": []string{"It works"},
			},
		},
		"risks":          []string{},
		"open_questions": []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func testReview(
	t *testing.T,
	runID, jobID, subjectPath, subjectHash string,
	verdict domain.Verdict,
	required bool,
) []byte {
	return testReviewForType(
		t,
		runID,
		jobID,
		"requirements",
		subjectPath,
		subjectHash,
		"application/json",
		verdict,
		required,
	)
}

func testReviewForType(
	t *testing.T,
	runID, jobID, reviewType, subjectPath, subjectHash, mediaType string,
	verdict domain.Verdict,
	required bool,
) []byte {
	t.Helper()
	findings := []any{}
	if required {
		findings = append(findings, map[string]any{
			"id":               "REV-001",
			"severity":         "high",
			"target":           "REQ-001",
			"problem":          "Too vague",
			"rationale":        "Cannot verify",
			"expected_outcome": "Make it observable",
		})
	}
	data, err := json.Marshal(map[string]any{
		"schema_version": "1.0",
		"run_id":         runID,
		"job_id":         jobID,
		"review_type":    reviewType,
		"subject": map[string]string{
			"path":       subjectPath,
			"sha256":     subjectHash,
			"media_type": mediaType,
		},
		"verdict": verdict,
		"summary": "Review",
		"scores": map[string]int{
			"clarity":      4,
			"completeness": 4,
			"feasibility":  4,
			"testability":  4,
			"risk_control": 4,
		},
		"required_changes":     findings,
		"optional_suggestions": []string{},
		"open_questions":       []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func requirementsPath(runID string, round int) string {
	return filepath.ToSlash(filepath.Join(
		".loop-engine",
		"runs",
		runID,
		"artifacts",
		"requirements",
		fmt.Sprintf("v%03d.json", round),
	))
}
