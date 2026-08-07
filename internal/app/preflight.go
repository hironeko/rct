package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hironeko/rct/internal/domain"
	"github.com/hironeko/rct/internal/store/filesystem"
)

const (
	InterruptionGitBootstrapRequired = "GIT_BOOTSTRAP_REQUIRED"
	InterruptionGitUnavailable       = "GIT_UNAVAILABLE"
	InterruptionDirtyWorktree        = "DIRTY_WORKTREE"
	InterruptionBaselineDrift        = "BASELINE_DRIFT"
	InterruptionProjectWriterBusy    = "PROJECT_WRITER_BUSY"
)

type ResumeOptions struct {
	Project string
	RunID   string
}

type preflightResult struct {
	RepositoryRoot  string
	ProjectRelative string
	BaselineCommit  string
}

type recoverablePreflightError struct {
	code        string
	summary     string
	remediation []string
}

func (e *recoverablePreflightError) Error() string { return e.summary }

func (s *Service) ExecuteImplementationPreflight(ctx context.Context, run domain.Run) (domain.Run, error) {
	if run.State != domain.StatePlanApproved && run.State != domain.StateImplementationPreflight {
		return run, fmt.Errorf("implementation preflight requires state %s; current state is %s", domain.StatePlanApproved, run.State)
	}
	store := filesystem.New(run.Project)
	if run.State != domain.StateImplementationPreflight {
		if err := s.transition(store, &run, domain.StateImplementationPreflight, "ImplementationPreflightStarted"); err != nil {
			return run, err
		}
	}
	lease, err := store.AcquireProjectWriterLease()
	if err != nil {
		if errors.Is(err, filesystem.ErrProjectWriterBusy) {
			return s.interruptPreflight(store, run, &recoverablePreflightError{
				code: InterruptionProjectWriterBusy, summary: "another rct run is writing this project",
				remediation: []string{"Wait for the active implementation to finish", "Run rct resume --project <path>"},
			})
		}
		return s.failRun(store, run, err)
	}
	defer lease.Close() //nolint:errcheck

	result, err := s.inspectImplementationPreflight(ctx, run, false)
	if err != nil {
		var recoverable *recoverablePreflightError
		if errors.As(err, &recoverable) {
			return s.interruptPreflight(store, run, recoverable)
		}
		return s.failRun(store, run, err)
	}
	run.RepositoryRoot = result.RepositoryRoot
	run.ProjectRelative = result.ProjectRelative
	run.BaseCommit = result.BaselineCommit
	run.PreflightCheckedAt = s.deps.Now().UTC()
	run.Interruption = nil
	run.WaitingReason = ""
	run.Failure = ""
	switch run.Mode {
	case domain.ModeSupervised:
		next := domain.StateAwaitingApproval
		event := "ImplementationPreflightPassed"
		if run.Approval != nil && run.Approval.SubjectSHA256 == run.PlanSHA256 &&
			run.Approval.BaselineCommit == run.BaseCommit {
			next = domain.StateImplementationReady
			event = "ImplementationPreflightResumedWithApproval"
		}
		if err := s.transition(store, &run, next, event); err != nil {
			return run, err
		}
	case domain.ModeAutonomous:
		if err := s.transition(store, &run, domain.StateImplementationReady, "ImplementationPreflightPassed"); err != nil {
			return run, err
		}
	case domain.ModeDesignOnly:
		return run, errors.New("design-only runs do not enter implementation preflight")
	default:
		return s.failRun(store, run, fmt.Errorf("unsupported run mode %q", run.Mode))
	}
	return run, nil
}

func (s *Service) Resume(ctx context.Context, options ResumeOptions) (domain.Run, error) {
	project, err := filepath.Abs(options.Project)
	if err != nil {
		return domain.Run{}, fmt.Errorf("resolve project path: %w", err)
	}
	store := filesystem.New(project)
	var run domain.Run
	if strings.TrimSpace(options.RunID) == "" {
		run, err = store.LoadCurrent()
	} else {
		run, err = store.Load(strings.TrimSpace(options.RunID))
	}
	if err != nil {
		return domain.Run{}, err
	}
	if (run.State == domain.StateAwaitingApproval || run.State == domain.StateImplementationReady) &&
		strings.TrimSpace(run.BaseCommit) == "" {
		if run.PlanPath == "" || run.PlanSHA256 == "" || run.ApprovalTargetHash != run.PlanSHA256 {
			return run, errors.New("legacy run cannot enter preflight because approved plan evidence is incomplete")
		}
		if run.State == domain.StateImplementationReady {
			run.Approval = nil
			run.ApprovalPath = ""
		}
		if err := s.transition(store, &run, domain.StateImplementationPreflight, "LegacyPreflightMigrationStarted"); err != nil {
			return run, err
		}
		return s.ExecuteImplementationPreflight(ctx, run)
	}
	if run.State != domain.StateWaitingForHuman || run.Interruption == nil {
		return run, errors.New("resume requires a preflight WAITING_FOR_HUMAN interruption")
	}
	switch run.Interruption.Code {
	case InterruptionGitBootstrapRequired, InterruptionGitUnavailable, InterruptionDirtyWorktree,
		InterruptionBaselineDrift, InterruptionProjectWriterBusy:
	default:
		return run, fmt.Errorf("resume does not support interruption code %q", run.Interruption.Code)
	}
	if run.Interruption.PlanSHA256 != run.PlanSHA256 {
		return run, errors.New("approved implementation plan changed while the run was waiting")
	}
	if err := s.transition(store, &run, domain.StateImplementationPreflight, "RunResumed"); err != nil {
		return run, err
	}
	return s.ExecuteImplementationPreflight(ctx, run)
}

func (s *Service) inspectImplementationPreflight(ctx context.Context, run domain.Run, requireExistingBaseline bool) (preflightResult, error) {
	if _, err := s.deps.LookPath("git"); err != nil {
		return preflightResult{}, &recoverablePreflightError{
			code: InterruptionGitUnavailable, summary: "git executable is unavailable",
			remediation: []string{"Install Git", "Run rct resume --project <path>"},
		}
	}
	class, err := s.classifyRepository(ctx, run.Project)
	if err != nil {
		return preflightResult{}, err
	}
	switch class {
	case RepositoryUnsafeBoundary:
		return preflightResult{}, errors.New("unsafe repository boundary: linked worktrees and submodules are not supported")
	case RepositoryUnmanaged, RepositoryManagedMinimal, RepositoryUnborn:
		return preflightResult{}, &recoverablePreflightError{
			code: InterruptionGitBootstrapRequired, summary: "Git bootstrap is required before implementation approval",
			remediation: []string{"Run rct init --project <path> --request-file <request.md> --yes", "Run rct resume --project <path>"},
		}
	}
	root, err := s.gitText(ctx, run.Project, "rev-parse", "--show-toplevel")
	if err != nil {
		return preflightResult{}, err
	}
	head, err := s.gitText(ctx, run.Project, "rev-parse", "HEAD")
	if err != nil {
		return preflightResult{}, &recoverablePreflightError{
			code: InterruptionGitBootstrapRequired, summary: "Git repository has no baseline commit",
			remediation: []string{"Run rct init --project <path> --adopt-existing --yes", "Run rct resume --project <path>"},
		}
	}
	if err := s.requireCleanImplementationWorktree(ctx, run.Project); err != nil {
		return preflightResult{}, &recoverablePreflightError{
			code: InterruptionDirtyWorktree, summary: err.Error(),
			remediation: []string{"Commit or remove project changes without using rct", "Run rct resume --project <path>"},
		}
	}
	root = strings.TrimSpace(root)
	head = strings.TrimSpace(head)
	if err := validateBootstrapReceipt(run.Project, root, head); err != nil {
		return preflightResult{}, &recoverablePreflightError{
			code: InterruptionBaselineDrift, summary: err.Error(),
			remediation: []string{"Inspect .rct/git-bootstrap-receipt.json and the Git history", "Restore a clean baseline before resuming"},
		}
	}
	if requireExistingBaseline && run.BaseCommit != "" && head != run.BaseCommit {
		return preflightResult{}, &recoverablePreflightError{
			code: InterruptionBaselineDrift, summary: "Git HEAD changed after implementation approval",
			remediation: []string{"Inspect the Git history", "Run rct resume to return to approval after restoring a clean baseline"},
		}
	}
	relative, err := filepath.Rel(root, run.Project)
	if err != nil {
		return preflightResult{}, err
	}
	return preflightResult{RepositoryRoot: root, ProjectRelative: filepath.ToSlash(relative), BaselineCommit: head}, nil
}

func validateBootstrapReceipt(project, repositoryRoot, head string) error {
	data, err := os.ReadFile(filepath.Join(project, ".rct", "git-bootstrap-receipt.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read Git bootstrap receipt: %w", err)
	}
	var receipt GitBootstrapReceipt
	if err := json.Unmarshal(data, &receipt); err != nil {
		return fmt.Errorf("decode Git bootstrap receipt: %w", err)
	}
	if receipt.RepositoryRoot != repositoryRoot || receipt.InitialCommit != head {
		return errors.New("Git bootstrap receipt no longer matches the repository baseline")
	}
	return nil
}

func (s *Service) interruptPreflight(store *filesystem.Store, run domain.Run, cause *recoverablePreflightError) (domain.Run, error) {
	if cause.code == InterruptionBaselineDrift {
		run.Approval = nil
		run.ApprovalPath = ""
		run.BaseCommit = ""
	}
	run.WaitingReason = cause.code + ": " + cause.summary
	run.Interruption = &domain.PreflightInterruption{
		Code: cause.code, Phase: "implementation_preflight", ResumeState: domain.StateImplementationPreflight,
		DetectedRevision: run.Revision, PlanSHA256: run.PlanSHA256, BaselineCommit: run.BaseCommit,
		Remediation: append([]string(nil), cause.remediation...), CreatedAt: s.deps.Now().UTC(),
	}
	if err := s.transition(store, &run, domain.StateWaitingForHuman, "ImplementationPreflightInterrupted"); err != nil {
		return run, errors.Join(cause, err)
	}
	return run, nil
}
