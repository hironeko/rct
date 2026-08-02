package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	agentassets "github.com/hironeko/loop-engine/agent-assets"
	"github.com/hironeko/loop-engine/internal/domain"
	"github.com/hironeko/loop-engine/internal/providers"
	loopruntime "github.com/hironeko/loop-engine/internal/runtime"
	"github.com/hironeko/loop-engine/internal/store/filesystem"
	"github.com/hironeko/loop-engine/schemas"
)

const maxUntrackedReviewBytes = 256 * 1024

type ImplementationOptions struct {
	MaxReviewRounds         int
	MaxVerificationAttempts int
}

type VerificationRecord struct {
	SchemaVersion string                      `json:"schema_version"`
	RunID         string                      `json:"run_id"`
	MilestoneID   string                      `json:"milestone_id"`
	Attempt       int                         `json:"attempt"`
	StartedAt     time.Time                   `json:"started_at"`
	FinishedAt    time.Time                   `json:"finished_at"`
	Passed        bool                        `json:"passed"`
	Commands      []VerificationCommandResult `json:"commands"`
}

type VerificationCommandResult struct {
	Executable string   `json:"executable"`
	Args       []string `json:"args"`
	ExitCode   int      `json:"exit_code"`
	Passed     bool     `json:"passed"`
	StdoutPath string   `json:"stdout_path"`
	StderrPath string   `json:"stderr_path"`
	Error      string   `json:"error,omitempty"`
}

func (s *Service) ExecuteImplementation(
	ctx context.Context,
	run domain.Run,
	options ImplementationOptions,
) (domain.Run, error) {
	if run.Backend != "direct" {
		return run, fmt.Errorf(
			"implementation currently requires backend=direct; selected backend is %q",
			run.Backend,
		)
	}
	if run.State != domain.StateImplementationReady {
		return run, fmt.Errorf(
			"implementation requires state %s; current state is %s",
			domain.StateImplementationReady,
			run.State,
		)
	}
	if options.MaxReviewRounds < 1 {
		return run, errors.New("max review rounds must be at least 1")
	}
	if options.MaxVerificationAttempts < 1 {
		return run, errors.New("max verification attempts must be at least 1")
	}
	if err := validateImplementationSeparation(run); err != nil {
		return run, err
	}
	for _, role := range []domain.Role{domain.RoleImplementer, domain.RoleReviewer} {
		binding := run.Roles[role]
		if err := s.deps.ProviderAuth(ctx, binding.Provider); err != nil {
			return s.failRun(
				filesystem.New(run.Project),
				run,
				fmt.Errorf("%s provider preflight failed: %w", role, err),
			)
		}
	}

	store := filesystem.New(run.Project)
	planBytes, err := store.ReadArtifact(run.ID, run.PlanPath)
	if err != nil {
		return s.failRun(store, run, err)
	}
	if sha256Hex(planBytes) != run.ApprovalTargetHash {
		return s.failRun(store, run, errors.New("implementation plan no longer matches authorized hash"))
	}
	if run.Mode == domain.ModeSupervised {
		if run.Approval == nil || run.Approval.SubjectSHA256 != run.ApprovalTargetHash {
			return s.failRun(store, run, errors.New("valid human implementation approval is missing"))
		}
	}
	plan, err := domain.ParseImplementationPlan(planBytes)
	if err != nil {
		return s.failRun(store, run, err)
	}
	if err := s.requireCleanImplementationWorktree(ctx, run.Project); err != nil {
		return s.failRun(store, run, err)
	}
	baseCommit, err := s.gitText(ctx, run.Project, "rev-parse", "HEAD")
	if err != nil {
		return s.failRun(store, run, fmt.Errorf("read implementation base commit: %w", err))
	}
	run.BaseCommit = strings.TrimSpace(baseCommit)
	run.MaxReviewRounds = options.MaxReviewRounds

	requirements, err := store.ReadArtifact(run.ID, run.RequirementsPath)
	if err != nil {
		return s.failRun(store, run, err)
	}
	architecture, err := store.ReadArtifact(run.ID, run.ArchitecturePath)
	if err != nil {
		return s.failRun(store, run, err)
	}
	implementationSchema, err := schemas.Implementation()
	if err != nil {
		return s.failRun(store, run, err)
	}
	reviewSchema, err := schemas.Review()
	if err != nil {
		return s.failRun(store, run, err)
	}
	implementerInstructions, err := agentassets.ImplementerInstructions()
	if err != nil {
		return s.failRun(store, run, err)
	}
	reviewerInstructions, err := agentassets.ReviewerInstructions()
	if err != nil {
		return s.failRun(store, run, err)
	}

	for milestoneIndex, milestone := range plan.Milestones {
		if err := ensureMilestoneDependencies(milestone, run.CompletedMilestones); err != nil {
			return s.failRun(store, run, err)
		}
		run.CurrentMilestone = milestoneIndex + 1
		run.CurrentMilestoneID = milestone.ID
		run.VerificationAttempts = 0
		var previousReview []byte
		var previousVerification []byte

		approved := false
		for round := 1; round <= options.MaxReviewRounds; round++ {
			run.ImplementationRound = round
			state := domain.StateMilestoneImplementation
			event := "MilestoneImplementationStarted"
			if round > 1 {
				state = domain.StateMilestoneFix
				event = "MilestoneFixStarted"
			}
			if err := s.transition(store, &run, state, event); err != nil {
				return run, err
			}

			implementerJobID := fmt.Sprintf("%s-r%02d-implementer", strings.ToLower(milestone.ID), round)
			implementationResult, jobErr := s.executeAgentJob(ctx, providers.Job{
				ID:       implementerJobID,
				Provider: run.Roles[domain.RoleImplementer].Provider,
				Role:     domain.RoleImplementer,
				Project:  run.Project,
				JobDir: filepath.Join(
					store.RunDir(run.ID), "jobs", implementerJobID,
				),
				Prompt: []byte(buildImplementationPrompt(
					implementerInstructions,
					run,
					milestone,
					requirements,
					architecture,
					planBytes,
					previousVerification,
					previousReview,
				)),
				Schema: implementationSchema,
				Access: providers.AccessWorkspaceWrite,
			})
			if jobErr != nil {
				return s.failRun(store, run, jobErr)
			}
			if err := validateImplementationResult(implementationResult.StructuredOutput, milestone.ID); err != nil {
				return s.failRun(store, run, err)
			}
			implementationPath, err := store.WriteRunFile(
				run.ID,
				fmt.Sprintf("artifacts/implementation/%s/v%03d.json", milestone.ID, round),
				implementationResult.StructuredOutput,
			)
			if err != nil {
				return s.failRun(store, run, err)
			}
			run.ImplementationPath = implementationPath
			if err := s.ensureBaseCommitUnchanged(ctx, run); err != nil {
				return s.failRun(store, run, err)
			}

			if err := s.transition(
				store,
				&run,
				domain.StateMilestoneVerification,
				"MilestoneVerificationStarted",
			); err != nil {
				return run, err
			}
			run.VerificationAttempts++
			verification, verificationBytes, verifyErr := s.verifyMilestone(
				ctx,
				store,
				run,
				milestone,
			)
			if verifyErr != nil {
				return s.failRun(store, run, verifyErr)
			}
			verificationPath, err := store.WriteRunFile(
				run.ID,
				fmt.Sprintf(
					"verification/%s/attempt-%03d.json",
					milestone.ID,
					run.VerificationAttempts,
				),
				verificationBytes,
			)
			if err != nil {
				return s.failRun(store, run, err)
			}
			run.VerificationPath = verificationPath
			if !verification.Passed {
				previousVerification = verificationBytes
				if run.VerificationAttempts >= options.MaxVerificationAttempts ||
					round == options.MaxReviewRounds {
					run.WaitingReason = "milestone verification limit reached: " + milestone.ID
					if err := s.transition(
						store,
						&run,
						domain.StateWaitingForHuman,
						"MilestoneVerificationLimitReached",
					); err != nil {
						return run, err
					}
					return run, nil
				}
				continue
			}
			previousVerification = nil
			if err := s.ensureBaseCommitUnchanged(ctx, run); err != nil {
				return s.failRun(store, run, err)
			}

			reviewSubject, err := s.buildCodeReviewSubject(
				ctx,
				run,
				milestone,
				requirements,
				architecture,
				planBytes,
				implementationResult.StructuredOutput,
				verificationBytes,
			)
			if err != nil {
				return s.failRun(store, run, err)
			}
			subjectPath, err := store.WriteRunFile(
				run.ID,
				fmt.Sprintf("review-subjects/%s/v%03d.md", milestone.ID, round),
				reviewSubject,
			)
			if err != nil {
				return s.failRun(store, run, err)
			}
			if err := s.transition(
				store,
				&run,
				domain.StateMilestoneReview,
				"MilestoneReviewStarted",
			); err != nil {
				return run, err
			}

			reviewerJobID := fmt.Sprintf("%s-r%02d-reviewer", strings.ToLower(milestone.ID), round)
			subjectHash := sha256Hex(withTrailingNewline(reviewSubject))
			reviewerResult, jobErr := s.executeAgentJob(ctx, providers.Job{
				ID:       reviewerJobID,
				Provider: run.Roles[domain.RoleReviewer].Provider,
				Role:     domain.RoleReviewer,
				Project:  run.Project,
				JobDir: filepath.Join(
					store.RunDir(run.ID), "jobs", reviewerJobID,
				),
				Prompt: []byte(buildCodeReviewPrompt(
					reviewerInstructions,
					run,
					milestone,
					reviewSubject,
					"code",
					reviewerJobID,
					subjectPath,
					subjectHash,
				)),
				Schema: reviewSchema,
				Access: providers.AccessReadOnly,
			})
			if jobErr != nil {
				return s.failRun(store, run, jobErr)
			}
			currentSubject, err := s.buildCodeReviewSubject(
				ctx,
				run,
				milestone,
				requirements,
				architecture,
				planBytes,
				implementationResult.StructuredOutput,
				verificationBytes,
			)
			if err != nil {
				return s.failRun(store, run, err)
			}
			if sha256Hex(withTrailingNewline(currentSubject)) != subjectHash {
				return s.failRun(store, run, errors.New("code changed after verification or during review"))
			}
			decision, err := domain.ParseReviewDecision(reviewerResult.StructuredOutput)
			if err != nil {
				return s.failRun(store, run, err)
			}
			if err := evaluateReviewGate(decision, reviewGateExpectation{
				RunID:       run.ID,
				JobID:       reviewerJobID,
				ReviewType:  "code",
				SubjectPath: subjectPath,
				SubjectHash: subjectHash,
				MediaType:   "text/markdown",
			}); err != nil {
				return s.failRun(store, run, err)
			}
			reviewPath, err := store.WriteRunFile(
				run.ID,
				fmt.Sprintf("reviews/code-%s-v%03d.json", milestone.ID, round),
				reviewerResult.StructuredOutput,
			)
			if err != nil {
				return s.failRun(store, run, err)
			}
			run.CodeReviewPath = reviewPath
			run.LastVerdict = decision.Verdict

			switch decision.Verdict {
			case domain.VerdictApproved:
				if err := s.transition(
					store,
					&run,
					domain.StateMilestoneApproved,
					"MilestoneApproved",
				); err != nil {
					return run, err
				}
				run.CompletedMilestones = append(run.CompletedMilestones, milestone.ID)
				approved = true
			case domain.VerdictChangesRequested:
				previousReview = reviewerResult.StructuredOutput
				if round == options.MaxReviewRounds {
					run.WaitingReason = "milestone review limit reached: " + milestone.ID
					if err := s.transition(
						store,
						&run,
						domain.StateWaitingForHuman,
						"MilestoneReviewLimitReached",
					); err != nil {
						return run, err
					}
					return run, nil
				}
			case domain.VerdictBlocked:
				run.WaitingReason = "milestone review blocked: " + milestone.ID
				if err := s.transition(
					store,
					&run,
					domain.StateBlocked,
					"MilestoneReviewBlocked",
				); err != nil {
					return run, err
				}
				return run, nil
			}
			if approved {
				break
			}
		}
		if !approved {
			return s.failRun(store, run, fmt.Errorf("milestone %s ended without approval", milestone.ID))
		}
	}

	return s.executeFinalGate(
		ctx,
		store,
		run,
		plan,
		requirements,
		architecture,
		planBytes,
		implementationSchema,
		reviewSchema,
		implementerInstructions,
		reviewerInstructions,
		options,
	)
}

func validateImplementationSeparation(run domain.Run) error {
	implementer, ok := run.Roles[domain.RoleImplementer]
	if !ok {
		return errors.New("implementer role binding is missing")
	}
	reviewer, ok := run.Roles[domain.RoleReviewer]
	if !ok {
		return errors.New("reviewer role binding is missing")
	}
	if implementer.Provider == reviewer.Provider || implementer.SessionID == reviewer.SessionID {
		return fmt.Errorf("%w: implementer and reviewer must use different providers and sessions", domain.ErrRoleAssignmentConflict)
	}
	return nil
}

func (s *Service) executeFinalGate(
	ctx context.Context,
	store *filesystem.Store,
	run domain.Run,
	plan domain.ImplementationPlan,
	requirements, architecture, planBytes, implementationSchema, reviewSchema []byte,
	implementerInstructions, reviewerInstructions string,
	options ImplementationOptions,
) (domain.Run, error) {
	var commands []domain.CommandSpec
	for _, milestone := range plan.Milestones {
		commands = append(commands, milestone.VerificationCommands...)
	}
	finalMilestone := domain.Milestone{
		ID:                   "FINAL",
		Objective:            "Verify and independently review the complete approved implementation",
		Scope:                []string{"All approved milestones and the cumulative Git diff"},
		AcceptanceCriteria:   []string{"All approved verification commands pass", "Final reviewer approves"},
		VerificationCommands: commands,
		DoneWhen:             []string{"Run reaches COMPLETED"},
	}
	finalSummary := []byte(`{"schema_version":"1.0","milestone_id":"FINAL","summary":"All approved milestones completed"}`)

	for round := 1; round <= options.MaxReviewRounds; round++ {
		run.CurrentMilestoneID = "FINAL"
		run.ImplementationRound = round
		run.VerificationAttempts = round
		if err := s.transition(
			store,
			&run,
			domain.StateFinalVerification,
			"FinalVerificationStarted",
		); err != nil {
			return run, err
		}
		verification, verificationBytes, err := s.verifyMilestone(
			ctx,
			store,
			run,
			finalMilestone,
		)
		if err != nil {
			return s.failRun(store, run, err)
		}
		verificationPath, err := store.WriteRunFile(
			run.ID,
			fmt.Sprintf("verification/final/attempt-%03d.json", round),
			verificationBytes,
		)
		if err != nil {
			return s.failRun(store, run, err)
		}
		run.VerificationPath = verificationPath
		if !verification.Passed {
			run.WaitingReason = "final verification failed"
			if err := s.transition(
				store,
				&run,
				domain.StateWaitingForHuman,
				"FinalVerificationFailed",
			); err != nil {
				return run, err
			}
			return run, nil
		}
		if err := s.ensureBaseCommitUnchanged(ctx, run); err != nil {
			return s.failRun(store, run, err)
		}
		subject, err := s.buildCodeReviewSubject(
			ctx,
			run,
			finalMilestone,
			requirements,
			architecture,
			planBytes,
			finalSummary,
			verificationBytes,
		)
		if err != nil {
			return s.failRun(store, run, err)
		}
		subjectPath, err := store.WriteRunFile(
			run.ID,
			fmt.Sprintf("review-subjects/final/v%03d.md", round),
			subject,
		)
		if err != nil {
			return s.failRun(store, run, err)
		}
		if err := s.transition(
			store,
			&run,
			domain.StateFinalReview,
			"FinalReviewStarted",
		); err != nil {
			return run, err
		}

		reviewerJobID := fmt.Sprintf("final-r%02d-reviewer", round)
		subjectHash := sha256Hex(withTrailingNewline(subject))
		reviewerResult, jobErr := s.executeAgentJob(ctx, providers.Job{
			ID:       reviewerJobID,
			Provider: run.Roles[domain.RoleReviewer].Provider,
			Role:     domain.RoleReviewer,
			Project:  run.Project,
			JobDir: filepath.Join(
				store.RunDir(run.ID), "jobs", reviewerJobID,
			),
			Prompt: []byte(buildCodeReviewPrompt(
				reviewerInstructions,
				run,
				finalMilestone,
				subject,
				"final",
				reviewerJobID,
				subjectPath,
				subjectHash,
			)),
			Schema: reviewSchema,
			Access: providers.AccessReadOnly,
		})
		if jobErr != nil {
			return s.failRun(store, run, jobErr)
		}
		currentSubject, err := s.buildCodeReviewSubject(
			ctx,
			run,
			finalMilestone,
			requirements,
			architecture,
			planBytes,
			finalSummary,
			verificationBytes,
		)
		if err != nil {
			return s.failRun(store, run, err)
		}
		if sha256Hex(withTrailingNewline(currentSubject)) != subjectHash {
			return s.failRun(store, run, errors.New("code changed during final review"))
		}
		decision, err := domain.ParseReviewDecision(reviewerResult.StructuredOutput)
		if err != nil {
			return s.failRun(store, run, err)
		}
		if err := evaluateReviewGate(decision, reviewGateExpectation{
			RunID:       run.ID,
			JobID:       reviewerJobID,
			ReviewType:  "final",
			SubjectPath: subjectPath,
			SubjectHash: subjectHash,
			MediaType:   "text/markdown",
		}); err != nil {
			return s.failRun(store, run, err)
		}
		reviewPath, err := store.WriteRunFile(
			run.ID,
			fmt.Sprintf("reviews/final-v%03d.json", round),
			reviewerResult.StructuredOutput,
		)
		if err != nil {
			return s.failRun(store, run, err)
		}
		run.CodeReviewPath = reviewPath
		run.LastVerdict = decision.Verdict

		switch decision.Verdict {
		case domain.VerdictApproved:
			run.CurrentMilestoneID = ""
			run.WaitingReason = ""
			if err := s.transition(store, &run, domain.StateCompleted, "RunCompleted"); err != nil {
				return run, err
			}
			return run, nil
		case domain.VerdictBlocked:
			run.WaitingReason = "final review blocked"
			if err := s.transition(store, &run, domain.StateBlocked, "FinalReviewBlocked"); err != nil {
				return run, err
			}
			return run, nil
		case domain.VerdictChangesRequested:
			if round == options.MaxReviewRounds {
				run.WaitingReason = "final review limit reached"
				if err := s.transition(
					store,
					&run,
					domain.StateWaitingForHuman,
					"FinalReviewLimitReached",
				); err != nil {
					return run, err
				}
				return run, nil
			}
		}

		lastMilestone := plan.Milestones[len(plan.Milestones)-1]
		lastMilestone.Objective = "Resolve required final review findings across the approved implementation"
		if err := s.transition(store, &run, domain.StateMilestoneFix, "FinalReviewFixStarted"); err != nil {
			return run, err
		}
		implementerJobID := fmt.Sprintf("final-r%02d-implementer", round+1)
		implementationResult, jobErr := s.executeAgentJob(ctx, providers.Job{
			ID:       implementerJobID,
			Provider: run.Roles[domain.RoleImplementer].Provider,
			Role:     domain.RoleImplementer,
			Project:  run.Project,
			JobDir: filepath.Join(
				store.RunDir(run.ID), "jobs", implementerJobID,
			),
			Prompt: []byte(buildImplementationPrompt(
				implementerInstructions,
				run,
				lastMilestone,
				requirements,
				architecture,
				planBytes,
				nil,
				reviewerResult.StructuredOutput,
			)),
			Schema: implementationSchema,
			Access: providers.AccessWorkspaceWrite,
		})
		if jobErr != nil {
			return s.failRun(store, run, jobErr)
		}
		if err := validateImplementationResult(implementationResult.StructuredOutput, lastMilestone.ID); err != nil {
			return s.failRun(store, run, err)
		}
		implementationPath, err := store.WriteRunFile(
			run.ID,
			fmt.Sprintf("artifacts/implementation/final/v%03d.json", round+1),
			implementationResult.StructuredOutput,
		)
		if err != nil {
			return s.failRun(store, run, err)
		}
		run.ImplementationPath = implementationPath
		finalSummary = implementationResult.StructuredOutput
		if err := s.ensureBaseCommitUnchanged(ctx, run); err != nil {
			return s.failRun(store, run, err)
		}
	}
	return s.failRun(store, run, errors.New("final review loop ended without a terminal state"))
}

func ensureMilestoneDependencies(milestone domain.Milestone, completed []string) error {
	set := make(map[string]bool, len(completed))
	for _, id := range completed {
		set[id] = true
	}
	for _, dependency := range milestone.Dependencies {
		if !set[dependency] {
			return fmt.Errorf("milestone %q dependency %q is not approved", milestone.ID, dependency)
		}
	}
	return nil
}

func validateImplementationResult(data []byte, milestoneID string) error {
	var result struct {
		SchemaVersion string `json:"schema_version"`
		MilestoneID   string `json:"milestone_id"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return fmt.Errorf("decode implementation result: %w", err)
	}
	if result.SchemaVersion != "1.0" || result.MilestoneID != milestoneID {
		return fmt.Errorf("implementation result milestone %q does not match %q", result.MilestoneID, milestoneID)
	}
	return nil
}

func (s *Service) verifyMilestone(
	ctx context.Context,
	store *filesystem.Store,
	run domain.Run,
	milestone domain.Milestone,
) (VerificationRecord, []byte, error) {
	record := VerificationRecord{
		SchemaVersion: "1.0",
		RunID:         run.ID,
		MilestoneID:   milestone.ID,
		Attempt:       run.VerificationAttempts,
		StartedAt:     s.deps.Now().UTC(),
		Passed:        true,
	}
	for index, command := range milestone.VerificationCommands {
		if err := domain.ValidateCommandSpec(command); err != nil {
			return record, nil, err
		}
		commandContext, cancel := context.WithTimeout(ctx, s.jobTimeout)
		result, runErr := s.runner.Run(commandContext, loopruntime.ProcessRequest{
			Executable: command.Executable,
			Args:       append([]string(nil), command.Args...),
			Directory:  run.Project,
		})
		cancel()
		stdoutPath, err := store.WriteRunFile(
			run.ID,
			fmt.Sprintf("verification/%s/attempt-%03d-command-%02d.stdout.log", milestone.ID, run.VerificationAttempts, index+1),
			result.Stdout,
		)
		if err != nil {
			return record, nil, err
		}
		stderrPath, err := store.WriteRunFile(
			run.ID,
			fmt.Sprintf("verification/%s/attempt-%03d-command-%02d.stderr.log", milestone.ID, run.VerificationAttempts, index+1),
			result.Stderr,
		)
		if err != nil {
			return record, nil, err
		}
		commandResult := VerificationCommandResult{
			Executable: command.Executable,
			Args:       append([]string(nil), command.Args...),
			ExitCode:   result.ExitCode,
			Passed:     runErr == nil && result.ExitCode == 0,
			StdoutPath: stdoutPath,
			StderrPath: stderrPath,
		}
		if runErr != nil {
			commandResult.Error = runErr.Error()
			record.Passed = false
		}
		record.Commands = append(record.Commands, commandResult)
		if !commandResult.Passed {
			break
		}
	}
	record.FinishedAt = s.deps.Now().UTC()
	encoded, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return record, nil, fmt.Errorf("encode verification result: %w", err)
	}
	return record, encoded, nil
}

func (s *Service) requireCleanImplementationWorktree(ctx context.Context, project string) error {
	status, err := s.gitText(ctx, project, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return fmt.Errorf("inspect git worktree: %w", err)
	}
	filtered := filterInternalStatus(status)
	if strings.TrimSpace(filtered) != "" {
		return fmt.Errorf("implementation requires a clean git worktree; changes found:\n%s", filtered)
	}
	return nil
}

func filterInternalStatus(status string) string {
	var lines []string
	for _, line := range strings.Split(status, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if len(line) < 3 {
			lines = append(lines, line)
			continue
		}
		path := strings.TrimSpace(line[2:])
		if path == ".loop-engine" || strings.HasPrefix(path, ".loop-engine/") {
			continue
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func (s *Service) ensureBaseCommitUnchanged(ctx context.Context, run domain.Run) error {
	current, err := s.gitText(ctx, run.Project, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	if strings.TrimSpace(current) != run.BaseCommit {
		return errors.New("git HEAD changed during implementation; commits are not authorized")
	}
	status, err := s.gitText(ctx, run.Project, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return err
	}
	for _, line := range strings.Split(filterInternalStatus(status), "\n") {
		if len(line) >= 2 && line[0] != ' ' && line[0] != '?' {
			return fmt.Errorf("git index changed during implementation: %s", line)
		}
	}
	return nil
}

func (s *Service) gitText(
	ctx context.Context,
	project string,
	args ...string,
) (string, error) {
	commandContext, cancel := context.WithTimeout(ctx, s.jobTimeout)
	defer cancel()
	result, err := s.runner.Run(commandContext, loopruntime.ProcessRequest{
		Executable: "git",
		Args:       args,
		Directory:  project,
	})
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(result.Stderr)))
	}
	return string(result.Stdout), nil
}

func (s *Service) buildCodeReviewSubject(
	ctx context.Context,
	run domain.Run,
	milestone domain.Milestone,
	requirements, architecture, plan []byte,
	implementationResult, verification []byte,
) ([]byte, error) {
	status, err := s.gitText(ctx, run.Project, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return nil, err
	}
	status = filterInternalStatus(status)
	if strings.TrimSpace(status) == "" {
		return nil, fmt.Errorf("milestone %s produced no reviewable source changes", milestone.ID)
	}
	diff, err := s.gitText(
		ctx,
		run.Project,
		"diff",
		"--no-ext-diff",
		"--binary",
		"--",
		".",
		":(exclude).loop-engine",
	)
	if err != nil {
		return nil, err
	}

	var subject strings.Builder
	fmt.Fprintf(&subject, "# Code Review Subject: %s\n\n", milestone.ID)
	fmt.Fprintf(&subject, "Base commit: `%s`\n\n", run.BaseCommit)
	subject.WriteString("## Milestone\n\n```json\n")
	milestoneJSON, _ := json.MarshalIndent(milestone, "", "  ")
	subject.Write(milestoneJSON)
	subject.WriteString("\n```\n\n## Approved requirements\n\n```json\n")
	subject.WriteString(prettyJSON(requirements))
	subject.WriteString("\n```\n\n## Approved architecture\n\n```json\n")
	subject.WriteString(prettyJSON(architecture))
	subject.WriteString("\n```\n\n## Approved implementation plan\n\n```json\n")
	subject.WriteString(prettyJSON(plan))
	subject.WriteString("\n```\n\n## Implementer result\n\n```json\n")
	subject.WriteString(prettyJSON(implementationResult))
	subject.WriteString("\n```\n\n## Verification\n\n```json\n")
	subject.WriteString(prettyJSON(verification))
	subject.WriteString("\n```\n\n## Git status\n\n```text\n")
	subject.WriteString(status)
	subject.WriteString("\n```\n\n## Git diff\n\n```diff\n")
	subject.WriteString(diff)
	subject.WriteString("\n```\n")

	for _, line := range strings.Split(status, "\n") {
		if !strings.HasPrefix(line, "?? ") {
			continue
		}
		relative := strings.TrimSpace(strings.TrimPrefix(line, "?? "))
		clean := filepath.Clean(relative)
		if clean == "." || filepath.IsAbs(clean) || clean == ".." ||
			strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return nil, fmt.Errorf("unsafe untracked path %q", relative)
		}
		path := filepath.Join(run.Project, clean)
		info, err := os.Lstat(path)
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() {
			continue
		}
		if info.Size() > maxUntrackedReviewBytes {
			fmt.Fprintf(&subject, "\n## Untracked file `%s`\n\nOmitted: file exceeds %d bytes.\n", filepath.ToSlash(clean), maxUntrackedReviewBytes)
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(&subject, "\n## Untracked file `%s`\n\n```text\n%s\n```\n", filepath.ToSlash(clean), data)
	}
	return []byte(subject.String()), nil
}

func buildImplementationPrompt(
	instructions string,
	run domain.Run,
	milestone domain.Milestone,
	requirements, architecture, plan, previousVerification, previousReview []byte,
) string {
	milestoneJSON, _ := json.MarshalIndent(milestone, "", "  ")
	var prompt strings.Builder
	fmt.Fprintf(&prompt, "%s\n\n# Job\n\nRun ID: %s\nMilestone: %s\nRound: %d\n", instructions, run.ID, milestone.ID, run.ImplementationRound)
	prompt.WriteString("\n## Approved milestone\n\n```json\n")
	prompt.Write(milestoneJSON)
	prompt.WriteString("\n```\n\n## Approved requirements\n\n```json\n")
	prompt.WriteString(prettyJSON(requirements))
	prompt.WriteString("\n```\n\n## Approved architecture\n\n```json\n")
	prompt.WriteString(prettyJSON(architecture))
	prompt.WriteString("\n```\n\n## Approved implementation plan\n\n```json\n")
	prompt.WriteString(prettyJSON(plan))
	prompt.WriteString("\n```\n")
	if len(previousVerification) > 0 {
		prompt.WriteString("\n## Failed authoritative verification\n\n```json\n")
		prompt.WriteString(prettyJSON(previousVerification))
		prompt.WriteString("\n```\n")
	}
	if len(previousReview) > 0 {
		prompt.WriteString("\n## Required code review feedback\n\n```json\n")
		prompt.WriteString(prettyJSON(previousReview))
		prompt.WriteString("\n```\n")
	}
	prompt.WriteString("\nImplement only this milestone now, then return only the JSON object required by the supplied schema.")
	return prompt.String()
}

func buildCodeReviewPrompt(
	instructions string,
	run domain.Run,
	milestone domain.Milestone,
	subject []byte,
	reviewType string,
	jobID, subjectPath, subjectHash string,
) string {
	return fmt.Sprintf(
		"%s\n\n# Job\n\nRun ID: %s\nJob ID: %s\nReview type: %s\nMilestone: %s\n"+
			"Subject path: %s\nSubject SHA-256: %s\nSubject media type: text/markdown\n\n%s\n\n"+
			"Copy the identifiers and subject fields exactly. Return only the JSON object required by the supplied schema.",
		instructions,
		run.ID,
		jobID,
		reviewType,
		milestone.ID,
		subjectPath,
		subjectHash,
		subject,
	)
}
