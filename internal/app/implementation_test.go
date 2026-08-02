package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hironeko/loop-engine/internal/domain"
	"github.com/hironeko/loop-engine/internal/providers"
	loopruntime "github.com/hironeko/loop-engine/internal/runtime"
	"github.com/hironeko/loop-engine/internal/store/filesystem"
)

type implementationGateway struct {
	implementations [][]byte
	verdicts        []domain.Verdict
	jobs            []providers.Job
}

func (g *implementationGateway) Execute(
	_ context.Context,
	job providers.Job,
) (providers.Result, error) {
	g.jobs = append(g.jobs, job)
	if job.Role == domain.RoleImplementer {
		if len(g.implementations) == 0 {
			return providers.Result{}, errors.New("no implementation output")
		}
		output := g.implementations[0]
		g.implementations = g.implementations[1:]
		return providers.Result{StructuredOutput: output}, nil
	}
	if len(g.verdicts) == 0 {
		return providers.Result{}, errors.New("no review verdict")
	}
	verdict := g.verdicts[0]
	g.verdicts = g.verdicts[1:]
	return providers.Result{StructuredOutput: codeReviewOutput(job.Prompt, verdict)}, nil
}

type implementationRunner struct {
	verificationFailures int
	verificationCalls    int
}

func (r *implementationRunner) Run(
	_ context.Context,
	request loopruntime.ProcessRequest,
) (loopruntime.ProcessResult, error) {
	if request.Executable == "git" {
		joined := strings.Join(request.Args, " ")
		switch {
		case strings.HasPrefix(joined, "status "):
			if r.verificationCalls == 0 {
				return loopruntime.ProcessResult{Stdout: []byte("?? .loop-engine/runs/internal\n")}, nil
			}
			return loopruntime.ProcessResult{Stdout: []byte(" M app.go\n")}, nil
		case joined == "rev-parse HEAD":
			return loopruntime.ProcessResult{Stdout: []byte("abc123\n")}, nil
		case strings.HasPrefix(joined, "diff "):
			return loopruntime.ProcessResult{Stdout: []byte("diff --git a/app.go b/app.go\n+implemented\n")}, nil
		}
	}
	if request.Executable == "go" {
		r.verificationCalls++
		if r.verificationCalls <= r.verificationFailures {
			return loopruntime.ProcessResult{
				Stderr:   []byte("test failed"),
				ExitCode: 1,
			}, errors.New("process exited with code 1")
		}
		return loopruntime.ProcessResult{Stdout: []byte("ok\n")}, nil
	}
	return loopruntime.ProcessResult{}, fmt.Errorf("unexpected command: %s %v", request.Executable, request.Args)
}

func TestExecuteImplementationRemediatesVerificationAndReview(t *testing.T) {
	t.Parallel()

	runner := &implementationRunner{verificationFailures: 1}
	gateway := &implementationGateway{
		implementations: [][]byte{
			testImplementationResult(t, "M01", "first"),
			testImplementationResult(t, "M01", "verification fix"),
			testImplementationResult(t, "M01", "review fix"),
		},
		verdicts: []domain.Verdict{
			domain.VerdictChangesRequested,
			domain.VerdictApproved,
		},
	}
	service, run := implementationFixture(t, gateway, runner)

	completed, err := service.ExecuteImplementation(context.Background(), run, ImplementationOptions{
		MaxReviewRounds:         3,
		MaxVerificationAttempts: 3,
	})
	if err != nil {
		t.Fatalf("ExecuteImplementation() error: %v", err)
	}
	if completed.State != domain.StateCompleted {
		t.Fatalf("state = %q, want %q", completed.State, domain.StateCompleted)
	}
	if completed.ImplementationRound != 3 {
		t.Fatalf("implementation round = %d, want 3", completed.ImplementationRound)
	}
	if len(completed.CompletedMilestones) != 1 || completed.CompletedMilestones[0] != "M01" {
		t.Fatalf("completed milestones = %#v", completed.CompletedMilestones)
	}
	reviewerJobs := 0
	for _, job := range gateway.jobs {
		switch job.Role {
		case domain.RoleImplementer:
			if job.Access != providers.AccessWorkspaceWrite {
				t.Fatalf("implementer access = %q", job.Access)
			}
		case domain.RoleReviewer:
			reviewerJobs++
			if job.Access != providers.AccessReadOnly {
				t.Fatalf("reviewer access = %q", job.Access)
			}
		}
	}
	if reviewerJobs != 2 {
		t.Fatalf("reviewer jobs = %d, want 2; failed verification must not be reviewed", reviewerJobs)
	}
	lastImplementerPrompt := gateway.jobs[len(gateway.jobs)-2].Prompt
	if !bytes.Contains(lastImplementerPrompt, []byte("Required code review feedback")) {
		t.Fatal("review remediation prompt does not contain required feedback")
	}
}

func TestExecuteImplementationStopsAtVerificationLimit(t *testing.T) {
	t.Parallel()

	runner := &implementationRunner{verificationFailures: 3}
	gateway := &implementationGateway{
		implementations: [][]byte{testImplementationResult(t, "M01", "first")},
	}
	service, run := implementationFixture(t, gateway, runner)
	stopped, err := service.ExecuteImplementation(context.Background(), run, ImplementationOptions{
		MaxReviewRounds:         3,
		MaxVerificationAttempts: 1,
	})
	if err != nil {
		t.Fatalf("ExecuteImplementation() error: %v", err)
	}
	if stopped.State != domain.StateWaitingForHuman {
		t.Fatalf("state = %q, want %q", stopped.State, domain.StateWaitingForHuman)
	}
	for _, job := range gateway.jobs {
		if job.Role == domain.RoleReviewer {
			t.Fatal("reviewer ran after failed verification")
		}
	}
}

func implementationFixture(
	t *testing.T,
	gateway providers.Gateway,
	runner loopruntime.ProcessRunner,
) (*Service, domain.Run) {
	t.Helper()
	project := t.TempDir()
	now := time.Date(2026, 8, 2, 15, 0, 0, 0, time.UTC)
	service := NewService(Dependencies{
		LookPath: func(name string) (string, error) { return "/usr/local/bin/" + name, nil },
		Getenv:   func(string) string { return "" },
		Now:      func() time.Time { return now },
		Random: bytes.NewReader([]byte{
			1, 2, 3, 4, 5, 6,
			7, 8, 9, 10, 11, 12,
		}),
		Agent:         gateway,
		ProviderAuth:  func(context.Context, domain.Provider) error { return nil },
		ProcessRunner: runner,
	})
	run, err := service.Start(context.Background(), StartOptions{
		Request:       "Build it",
		Project:       project,
		Mode:          "supervised",
		Backend:       "direct",
		SkipToolCheck: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	store := filesystem.New(project)
	requirementsPath, err := store.WriteRunFile(run.ID, "artifacts/requirements/v001.json", testRequirements(t, "Approved"))
	if err != nil {
		t.Fatal(err)
	}
	architecturePath, err := store.WriteRunFile(run.ID, "artifacts/architecture/v001.json", testArchitecture(t))
	if err != nil {
		t.Fatal(err)
	}
	plan := withTrailingNewline(testPlan(t))
	planPath, err := store.WriteRunFile(run.ID, "artifacts/plan/v001.json", plan)
	if err != nil {
		t.Fatal(err)
	}
	run.RequirementsPath = requirementsPath
	run.ArchitecturePath = architecturePath
	run.PlanPath = planPath
	run.PlanReview = "reviews/plan-v001.json"
	run.PlanSHA256 = sha256Hex(plan)
	run.ApprovalTargetHash = run.PlanSHA256
	run.LastVerdict = domain.VerdictApproved
	run.Approval = &domain.HumanApprovalRecord{
		SchemaVersion: "1.0",
		ID:            "approval_test",
		RunID:         run.ID,
		GateKind:      "implementation_start",
		Phase:         "implementation",
		SubjectPath:   run.PlanPath,
		SubjectSHA256: run.PlanSHA256,
		Approver:      "tester",
		CreatedAt:     now,
		ConsumedAt:    now,
		StateRevision: run.Revision,
	}
	previous := run.State
	run.State = domain.StateImplementationReady
	run.Revision++
	if err := store.Update(run, previous, "HumanImplementationApprovalConsumed"); err != nil {
		t.Fatal(err)
	}
	return service, run
}

func testImplementationResult(t *testing.T, milestoneID, summary string) []byte {
	t.Helper()
	data, err := json.Marshal(map[string]any{
		"schema_version":         "1.0",
		"milestone_id":           milestoneID,
		"summary":                summary,
		"changed_files":          []string{"app.go"},
		"tests_added_or_updated": []string{"app_test.go"},
		"commands_run":           []string{"go test ./..."},
		"known_issues":           []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func promptField(prompt []byte, name string) string {
	prefix := name + ": "
	for _, line := range strings.Split(string(prompt), "\n") {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return ""
}

func codeReviewOutput(prompt []byte, verdict domain.Verdict) []byte {
	findings := []any{}
	if verdict == domain.VerdictChangesRequested {
		findings = append(findings, map[string]any{
			"id": "CODE-001", "severity": "high", "target": "app.go",
			"problem": "Needs correction", "rationale": "Test gap",
			"expected_outcome": "Add the missing behavior",
		})
	}
	data, _ := json.Marshal(map[string]any{
		"schema_version": "1.0",
		"run_id":         promptField(prompt, "Run ID"),
		"job_id":         promptField(prompt, "Job ID"),
		"review_type":    "code",
		"subject": map[string]string{
			"path":       promptField(prompt, "Subject path"),
			"sha256":     promptField(prompt, "Subject SHA-256"),
			"media_type": "text/markdown",
		},
		"verdict": verdict,
		"summary": "Code review",
		"scores": map[string]int{
			"clarity": 4, "completeness": 4, "feasibility": 4,
			"testability": 4, "risk_control": 4,
		},
		"required_changes":     findings,
		"optional_suggestions": []string{},
		"open_questions":       []string{},
	})
	return data
}
