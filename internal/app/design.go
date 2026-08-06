package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	agentassets "github.com/hironeko/rct/agent-assets"
	"github.com/hironeko/rct/internal/domain"
	"github.com/hironeko/rct/internal/providers"
	"github.com/hironeko/rct/internal/store/filesystem"
	"github.com/hironeko/rct/schemas"
)

func (s *Service) ExecuteDesign(
	ctx context.Context,
	run domain.Run,
	maxReviewRounds int,
) (domain.Run, error) {
	if run.Backend != "direct" {
		return run, fmt.Errorf(
			"design execution currently requires backend=direct; selected backend is %q",
			run.Backend,
		)
	}
	if run.State != domain.StateIntake {
		return run, fmt.Errorf(
			"design execution requires state %s; current state is %s",
			domain.StateIntake,
			run.State,
		)
	}
	if err := validateIndependentReview(run); err != nil {
		return run, err
	}
	if maxReviewRounds < 1 {
		return run, errors.New("max review rounds must be at least 1")
	}
	for _, role := range []domain.Role{domain.RoleDesigner, domain.RoleReviewer} {
		binding := run.Roles[role]
		if err := s.deps.ProviderAuth(ctx, binding.Provider); err != nil {
			return s.failRun(
				filesystem.New(run.Project),
				run,
				fmt.Errorf("%s provider preflight failed: %w", role, err),
			)
		}
	}
	run.MaxReviewRounds = maxReviewRounds

	store := filesystem.New(run.Project)
	request, err := store.LoadRequest(run.ID)
	if err != nil {
		return s.failRun(store, run, err)
	}
	requirementsSchema, err := schemas.Requirements()
	if err != nil {
		return s.failRun(store, run, err)
	}
	reviewSchema, err := schemas.Review()
	if err != nil {
		return s.failRun(store, run, err)
	}
	designerInstructions, err := agentassets.DesignerInstructions()
	if err != nil {
		return s.failRun(store, run, err)
	}
	reviewerInstructions, err := agentassets.ReviewerInstructions()
	if err != nil {
		return s.failRun(store, run, err)
	}

	var previousArtifact []byte
	var previousReview []byte
	for round := 1; round <= run.MaxReviewRounds; round++ {
		run.RequirementsRound = round
		if err := s.transition(
			store,
			&run,
			domain.StateRequirementsDraft,
			"RequirementsDraftingStarted",
		); err != nil {
			return run, err
		}

		designerJobID := fmt.Sprintf("requirements-r%02d-designer", round)
		designerResult, jobErr := s.executeAgentJob(ctx, providers.Job{
			ID:       designerJobID,
			Provider: run.Roles[domain.RoleDesigner].Provider,
			Role:     domain.RoleDesigner,
			Project:  run.Project,
			JobDir: filepath.Join(
				store.RunDir(run.ID),
				"jobs",
				designerJobID,
			),
			Prompt: []byte(buildDesignerPrompt(
				designerInstructions,
				request,
				previousArtifact,
				previousReview,
				round,
			)),
			Schema: requirementsSchema,
		})
		if jobErr != nil {
			return s.failRun(store, run, jobErr)
		}
		requirementsPath, err := store.WriteRunFile(
			run.ID,
			fmt.Sprintf("artifacts/requirements/v%03d.json", round),
			withTrailingNewline(designerResult.StructuredOutput),
		)
		if err != nil {
			return s.failRun(store, run, err)
		}
		run.RequirementsPath = requirementsPath
		if err := s.transition(
			store,
			&run,
			domain.StateRequirementsReview,
			"RequirementsArtifactProduced",
		); err != nil {
			return run, err
		}

		reviewerJobID := fmt.Sprintf("requirements-r%02d-reviewer", round)
		subjectHash := sha256Hex(withTrailingNewline(designerResult.StructuredOutput))
		reviewerResult, jobErr := s.executeAgentJob(ctx, providers.Job{
			ID:       reviewerJobID,
			Provider: run.Roles[domain.RoleReviewer].Provider,
			Role:     domain.RoleReviewer,
			Project:  run.Project,
			JobDir: filepath.Join(
				store.RunDir(run.ID),
				"jobs",
				reviewerJobID,
			),
			Prompt: []byte(buildReviewerPrompt(
				reviewerInstructions,
				request,
				designerResult.StructuredOutput,
				round,
				run.ID,
				reviewerJobID,
				requirementsPath,
				subjectHash,
				"application/json",
			)),
			Schema: reviewSchema,
		})
		if jobErr != nil {
			return s.failRun(store, run, jobErr)
		}
		reviewPath, err := store.WriteRunFile(
			run.ID,
			fmt.Sprintf("reviews/requirements-v%03d.json", round),
			reviewerResult.StructuredOutput,
		)
		if err != nil {
			return s.failRun(store, run, err)
		}
		decision, err := domain.ParseReviewDecision(reviewerResult.StructuredOutput)
		if err != nil {
			return s.failRun(store, run, err)
		}
		if err := evaluateReviewGate(
			decision,
			reviewGateExpectation{
				RunID:       run.ID,
				JobID:       reviewerJobID,
				ReviewType:  "requirements",
				SubjectPath: requirementsPath,
				SubjectHash: subjectHash,
				MediaType:   "application/json",
			},
		); err != nil {
			return s.failRun(store, run, err)
		}
		run.RequirementsReview = reviewPath
		run.LastVerdict = decision.Verdict

		switch decision.Verdict {
		case domain.VerdictApproved:
			if err := s.transition(
				store,
				&run,
				domain.StateRequirementsApproved,
				"RequirementsApproved",
			); err != nil {
				return run, err
			}
			return run, nil
		case domain.VerdictBlocked:
			if err := s.transition(
				store,
				&run,
				domain.StateBlocked,
				"RequirementsReviewBlocked",
			); err != nil {
				return run, err
			}
			return run, nil
		case domain.VerdictChangesRequested:
			previousArtifact = designerResult.StructuredOutput
			previousReview = reviewerResult.StructuredOutput
			if round == run.MaxReviewRounds {
				if err := s.transition(
					store,
					&run,
					domain.StateWaitingForHuman,
					"RequirementsReviewLimitReached",
				); err != nil {
					return run, err
				}
				return run, nil
			}
		default:
			return s.failRun(
				store,
				run,
				fmt.Errorf("unsupported review verdict %q", decision.Verdict),
			)
		}
	}
	return s.failRun(store, run, errors.New("requirements loop ended without a terminal state"))
}

func (s *Service) executeAgentJob(
	ctx context.Context,
	job providers.Job,
) (providers.Result, error) {
	jobContext, cancel := context.WithTimeout(ctx, s.jobTimeout)
	defer cancel()

	runID := filepath.Base(filepath.Dir(filepath.Dir(job.JobDir)))
	store := filesystem.New(job.Project)
	run, err := store.Load(runID)
	if err != nil {
		return providers.Result{}, fmt.Errorf("load run for job progress: %w", err)
	}
	phase, round := jobProgressIdentity(job.ID)
	now := s.deps.Now().UTC()
	activity := domain.CurrentActivity{
		RunID: runID, Status: domain.ActivityQueued, Phase: phase, Action: jobAction(job.Role),
		Role: string(job.Role), Provider: string(job.Provider), Backend: run.Backend, JobID: job.ID,
		Round: round, MaxRounds: run.MaxReviewRounds, ArtifactKind: phase, CandidateVersion: round,
		PreviousVerdict: string(run.LastVerdict), StartedAt: now, LastHeartbeatAt: now,
	}
	if _, err := store.WriteActivity(activity); err != nil {
		return providers.Result{}, err
	}
	if _, err := store.AppendProgressEvent(runID, progressEventForJob(run, activity, "JobQueued", now)); err != nil {
		return providers.Result{}, err
	}
	activity.Status = domain.ActivityRunning
	if _, err := store.WriteActivity(activity); err != nil {
		return providers.Result{}, err
	}
	if _, err := store.AppendProgressEvent(runID, progressEventForJob(run, activity, "JobStarted", now)); err != nil {
		return providers.Result{}, err
	}

	var mu sync.Mutex
	var callbackErr error
	lastOutputPersist := time.Time{}
	update := func(at time.Time, output bool) {
		mu.Lock()
		defer mu.Unlock()
		if callbackErr != nil {
			return
		}
		if output && !lastOutputPersist.IsZero() && at.Sub(lastOutputPersist) < time.Second {
			return
		}
		activity.LastHeartbeatAt = at.UTC()
		if _, writeErr := store.WriteActivity(activity); writeErr != nil {
			callbackErr = fmt.Errorf("persist job activity: %w", writeErr)
			cancel()
			return
		}
		if output {
			lastOutputPersist = at
		}
	}
	job.OnHeartbeat = func(at time.Time) { update(at, false) }
	job.OnOutput = func(_ string, at time.Time) { update(at, true) }
	result, jobErr := s.agent.Execute(jobContext, job)

	mu.Lock()
	observabilityErr := callbackErr
	activity.LastHeartbeatAt = s.deps.Now().UTC()
	mu.Unlock()
	if observabilityErr != nil && jobErr == nil {
		jobErr = observabilityErr
	}
	completedAt := s.deps.Now().UTC()
	if jobErr != nil {
		activity.Status = domain.ActivityFailed
		activity.Error = &domain.SafeProgressError{Code: "PROVIDER_JOB_FAILED", Summary: "Provider job ended before a valid result was produced", Retryable: true, NextAction: "Inspect the local job directory, then run rct status"}
		_, _ = store.WriteActivity(activity)
		_, _ = store.AppendProgressEvent(runID, progressEventForJob(run, activity, "JobFailed", completedAt))
		return result, jobErr
	}
	activity.Status = domain.ActivityCompleted
	activity.Error = nil
	if _, err := store.WriteActivity(activity); err != nil {
		return result, err
	}
	if _, err := store.AppendProgressEvent(runID, progressEventForJob(run, activity, "JobCompleted", completedAt)); err != nil {
		return result, err
	}
	return result, nil
}

func jobProgressIdentity(jobID string) (string, int) {
	phase := "implementation"
	for _, candidate := range []string{"requirements", "architecture", "plan", "final"} {
		if strings.HasPrefix(strings.ToLower(jobID), candidate+"-") {
			phase = candidate
			if candidate == "final" {
				phase = "final_review"
			}
			break
		}
	}
	round := 0
	for _, part := range strings.Split(jobID, "-") {
		if len(part) > 1 && part[0] == 'r' {
			if value, err := strconv.Atoi(part[1:]); err == nil {
				round = value
				break
			}
		}
	}
	return phase, round
}

func jobAction(role domain.Role) string {
	switch role {
	case domain.RoleDesigner:
		return "drafting"
	case domain.RoleReviewer:
		return "reviewing"
	case domain.RoleImplementer:
		return "implementing"
	default:
		return "running"
	}
}

func progressEventForJob(run domain.Run, activity domain.CurrentActivity, kind string, at time.Time) domain.ProgressEvent {
	return domain.ProgressEvent{
		Timestamp: at, RunID: run.ID, Type: kind, StateAfter: string(run.State), Phase: activity.Phase,
		Role: activity.Role, Provider: activity.Provider, Backend: activity.Backend, JobID: activity.JobID,
		Round: activity.Round, ArtifactKind: activity.ArtifactKind, Version: activity.CandidateVersion,
	}
}

func (s *Service) transition(
	store *filesystem.Store,
	run *domain.Run,
	state domain.WorkflowState,
	event string,
) error {
	previous := run.State
	run.State = state
	run.UpdatedAt = s.deps.Now().UTC()
	run.Revision++
	return store.Update(*run, previous, event)
}

func (s *Service) failRun(
	store *filesystem.Store,
	run domain.Run,
	cause error,
) (domain.Run, error) {
	run.Failure = cause.Error()
	if transitionErr := s.transition(store, &run, domain.StateFailed, "RunFailed"); transitionErr != nil {
		return run, errors.Join(cause, transitionErr)
	}
	return run, cause
}

func validateIndependentReview(run domain.Run) error {
	designer, ok := run.Roles[domain.RoleDesigner]
	if !ok {
		return errors.New("designer role binding is missing")
	}
	reviewer, ok := run.Roles[domain.RoleReviewer]
	if !ok {
		return errors.New("reviewer role binding is missing")
	}
	if designer.Provider == reviewer.Provider {
		return fmt.Errorf(
			"%w: designer and reviewer both use %q",
			domain.ErrRoleAssignmentConflict,
			designer.Provider,
		)
	}
	if designer.SessionID == reviewer.SessionID {
		return fmt.Errorf(
			"%w: designer and reviewer session ids must differ",
			domain.ErrRoleAssignmentConflict,
		)
	}
	return nil
}

func buildDesignerPrompt(
	instructions string,
	request, previousArtifact, previousReview []byte,
	round int,
) string {
	var prompt strings.Builder
	fmt.Fprintf(&prompt, "%s\n\n# Job\n\nRequirements round: %d\n", instructions, round)
	prompt.WriteString("\n## Original request\n\n")
	prompt.Write(request)
	if len(previousArtifact) > 0 {
		prompt.WriteString("\n\n## Previous requirements artifact\n\n```json\n")
		prompt.WriteString(prettyJSON(previousArtifact))
		prompt.WriteString("\n```\n")
	}
	if len(previousReview) > 0 {
		prompt.WriteString("\n\n## Required review feedback\n\n```json\n")
		prompt.WriteString(prettyJSON(previousReview))
		prompt.WriteString("\n```\n")
	}
	prompt.WriteString(
		"\n\nProduce the requirements artifact now. Return only the JSON object required by the supplied schema.",
	)
	return prompt.String()
}

func buildReviewerPrompt(
	instructions string,
	request, artifact []byte,
	round int,
	runID, jobID, subjectPath, subjectHash, subjectMediaType string,
) string {
	return fmt.Sprintf(
		"%s\n\n# Job\n\nRun ID: %s\nJob ID: %s\nRequirements review round: %d\n"+
			"Subject path: %s\nSubject SHA-256: %s\nSubject media type: %s\n\n## Original request\n\n%s\n\n"+
			"## Requirements artifact under review\n\n```json\n%s\n```\n\n"+
			"Copy the Run ID, Job ID, subject path, subject SHA-256, and subject media type exactly into the review. "+
			"Review independently and return only the JSON object required by the supplied schema.",
		instructions,
		runID,
		jobID,
		round,
		subjectPath,
		subjectHash,
		subjectMediaType,
		request,
		prettyJSON(artifact),
	)
}

func withTrailingNewline(data []byte) []byte {
	if len(data) > 0 && data[len(data)-1] == '\n' {
		return data
	}
	result := make([]byte, len(data)+1)
	copy(result, data)
	result[len(result)-1] = '\n'
	return result
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func prettyJSON(data []byte) string {
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return string(data)
	}
	formatted, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return string(data)
	}
	return string(formatted)
}
