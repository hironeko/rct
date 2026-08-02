package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hironeko/loop-engine/internal/domain"
	"github.com/hironeko/loop-engine/internal/store/filesystem"
)

func TestApproveRecordsAndConsumesPlanAuthorization(t *testing.T) {
	t.Parallel()

	service, run := approvalFixture(t)
	approvedRevision := run.Revision
	approved, err := service.Approve(context.Background(), ApproveOptions{
		Project:          run.Project,
		Approver:         "hironeko",
		Note:             "Proceed",
		ExpectedRevision: approvedRevision,
	})
	if err != nil {
		t.Fatalf("Approve() error: %v", err)
	}
	if approved.State != domain.StateImplementationReady {
		t.Fatalf("state = %q, want %q", approved.State, domain.StateImplementationReady)
	}
	if approved.Approval == nil || approved.Approval.SubjectSHA256 != approved.PlanSHA256 {
		t.Fatalf("approval = %#v", approved.Approval)
	}
	if approved.Approval.StateRevision != approvedRevision {
		t.Fatalf("approval revision = %d, want %d", approved.Approval.StateRevision, approvedRevision)
	}
	if _, err := os.Stat(filepath.Join(
		run.Project,
		filepath.FromSlash(approved.ApprovalPath),
	)); err != nil {
		t.Fatalf("approval record missing: %v", err)
	}
	if _, err := service.Approve(context.Background(), ApproveOptions{
		Project: run.Project, Approver: "hironeko",
	}); err == nil || !strings.Contains(err.Error(), "requires state") {
		t.Fatalf("second Approve() error = %v, want state rejection", err)
	}
}

func TestApproveRejectsChangedPlanHash(t *testing.T) {
	t.Parallel()

	service, run := approvalFixture(t)
	if _, err := filesystem.New(run.Project).WriteRunFile(
		run.ID,
		"artifacts/plan/v001.json",
		[]byte(`{"schema_version":"1.0","milestones":[]}`),
	); err != nil {
		t.Fatal(err)
	}
	approved, err := service.Approve(context.Background(), ApproveOptions{
		Project: run.Project, Approver: "hironeko",
	})
	if err == nil || !strings.Contains(err.Error(), "does not match approval target") {
		t.Fatalf("Approve() error = %v, want stale hash rejection", err)
	}
	if approved.State != domain.StateAwaitingApproval {
		t.Fatalf("state = %q, want unchanged awaiting state", approved.State)
	}
}

func TestApproveDoesNotOverrideReviewLimit(t *testing.T) {
	t.Parallel()

	service, run := approvalFixture(t)
	store := filesystem.New(run.Project)
	run.State = domain.StateWaitingForHuman
	run.WaitingReason = "plan review limit reached"
	run.Revision++
	if err := store.Update(run, domain.StateAwaitingApproval, "ReviewLimitInjected"); err != nil {
		t.Fatal(err)
	}
	_, err := service.Approve(context.Background(), ApproveOptions{
		Project: run.Project, Approver: "hironeko",
	})
	if err == nil || !strings.Contains(err.Error(), "requires state") {
		t.Fatalf("Approve() error = %v, want non-override rejection", err)
	}
}

func approvalFixture(t *testing.T) (*Service, domain.Run) {
	t.Helper()
	project := t.TempDir()
	service := NewService(Dependencies{
		LookPath: func(name string) (string, error) { return "/usr/local/bin/" + name, nil },
		Getenv:   func(string) string { return "" },
		Now: func() time.Time {
			return time.Date(2026, 8, 2, 14, 0, 0, 0, time.UTC)
		},
		Random: bytes.NewReader([]byte{
			1, 2, 3, 4, 5, 6,
			7, 8, 9, 10, 11, 12,
		}),
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
	plan := withTrailingNewline(testPlan(t))
	store := filesystem.New(project)
	planPath, err := store.WriteRunFile(run.ID, "artifacts/plan/v001.json", plan)
	if err != nil {
		t.Fatal(err)
	}
	run.PlanPath = planPath
	run.PlanReview = filepath.ToSlash(filepath.Join(
		".loop-engine", "runs", run.ID, "reviews", "plan-v001.json",
	))
	run.PlanSHA256 = sha256Hex(plan)
	run.ApprovalTargetHash = run.PlanSHA256
	run.LastVerdict = domain.VerdictApproved
	previous := run.State
	run.State = domain.StateAwaitingApproval
	run.Revision++
	if err := store.Update(run, previous, "ImplementationApprovalRequested"); err != nil {
		t.Fatal(err)
	}
	return service, run
}
