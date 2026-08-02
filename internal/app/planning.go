package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	agentassets "github.com/hironeko/loop-engine/agent-assets"
	"github.com/hironeko/loop-engine/internal/domain"
	"github.com/hironeko/loop-engine/internal/providers"
	"github.com/hironeko/loop-engine/internal/store/filesystem"
	"github.com/hironeko/loop-engine/schemas"
)

type documentPhase struct {
	kind          string
	producerState domain.WorkflowState
	reviewState   domain.WorkflowState
	approvedState domain.WorkflowState
	startEvent    string
	producedEvent string
	approvedEvent string
	blockedEvent  string
	limitEvent    string
	schema        []byte
	reviewSchema  []byte
	input         []byte
}

// ExecutePlanning creates and independently reviews Architecture and
// Implementation Plan artifacts after Requirements approval.
func (s *Service) ExecutePlanning(
	ctx context.Context,
	run domain.Run,
	maxReviewRounds int,
) (domain.Run, error) {
	if run.Backend != "direct" {
		return run, fmt.Errorf(
			"planning execution currently requires backend=direct; selected backend is %q",
			run.Backend,
		)
	}
	if run.State != domain.StateRequirementsApproved {
		return run, fmt.Errorf(
			"planning execution requires state %s; current state is %s",
			domain.StateRequirementsApproved,
			run.State,
		)
	}
	if err := validateIndependentReview(run); err != nil {
		return run, err
	}
	if maxReviewRounds < 1 {
		return run, errors.New("max review rounds must be at least 1")
	}
	run.MaxReviewRounds = maxReviewRounds
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

	store := filesystem.New(run.Project)
	request, err := store.LoadRequest(run.ID)
	if err != nil {
		return s.failRun(store, run, err)
	}
	requirements, err := store.ReadArtifact(run.ID, run.RequirementsPath)
	if err != nil {
		return s.failRun(store, run, err)
	}
	architectureSchema, err := schemas.Architecture()
	if err != nil {
		return s.failRun(store, run, err)
	}
	planSchema, err := schemas.Plan()
	if err != nil {
		return s.failRun(store, run, err)
	}
	reviewSchema, err := schemas.Review()
	if err != nil {
		return s.failRun(store, run, err)
	}

	architectureInput := joinPlanningInputs(
		"Original request", request,
		"Approved requirements", requirements,
	)
	run, err = s.executeDocumentPhase(ctx, store, run, maxReviewRounds, documentPhase{
		kind:          "architecture",
		producerState: domain.StateArchitectureDraft,
		reviewState:   domain.StateArchitectureReview,
		approvedState: domain.StateArchitectureApproved,
		startEvent:    "ArchitectureDraftingStarted",
		producedEvent: "ArchitectureArtifactProduced",
		approvedEvent: "ArchitectureApproved",
		blockedEvent:  "ArchitectureReviewBlocked",
		limitEvent:    "ArchitectureReviewLimitReached",
		schema:        architectureSchema,
		reviewSchema:  reviewSchema,
		input:         architectureInput,
	})
	if err != nil || run.State != domain.StateArchitectureApproved {
		return run, err
	}

	architecture, err := store.ReadArtifact(run.ID, run.ArchitecturePath)
	if err != nil {
		return s.failRun(store, run, err)
	}
	planInput := joinPlanningInputs(
		"Original request", request,
		"Approved requirements", requirements,
		"Approved architecture", architecture,
	)
	run, err = s.executeDocumentPhase(ctx, store, run, maxReviewRounds, documentPhase{
		kind:          "plan",
		producerState: domain.StatePlanDraft,
		reviewState:   domain.StatePlanReview,
		approvedState: domain.StatePlanApproved,
		startEvent:    "PlanDraftingStarted",
		producedEvent: "PlanArtifactProduced",
		approvedEvent: "PlanApproved",
		blockedEvent:  "PlanReviewBlocked",
		limitEvent:    "PlanReviewLimitReached",
		schema:        planSchema,
		reviewSchema:  reviewSchema,
		input:         planInput,
	})
	if err != nil || run.State != domain.StatePlanApproved {
		return run, err
	}

	plan, err := store.ReadArtifact(run.ID, run.PlanPath)
	if err != nil {
		return s.failRun(store, run, err)
	}
	if _, err := domain.ParseImplementationPlan(plan); err != nil {
		return s.failRun(store, run, err)
	}
	run.PlanSHA256 = sha256Hex(plan)
	run.ApprovalTargetHash = run.PlanSHA256
	switch run.Mode {
	case domain.ModeSupervised:
		if err := s.transition(
			store,
			&run,
			domain.StateAwaitingApproval,
			"ImplementationApprovalRequested",
		); err != nil {
			return run, err
		}
	case domain.ModeAutonomous:
		if err := s.transition(
			store,
			&run,
			domain.StateImplementationReady,
			"ImplementationAuthorizedByMode",
		); err != nil {
			return run, err
		}
	case domain.ModeDesignOnly:
		if err := s.transition(
			store,
			&run,
			domain.StatePlanApproved,
			"DesignOnlyPlanningCompleted",
		); err != nil {
			return run, err
		}
	default:
		return s.failRun(store, run, fmt.Errorf("unsupported run mode %q", run.Mode))
	}
	return run, nil
}

func (s *Service) executeDocumentPhase(
	ctx context.Context,
	store *filesystem.Store,
	run domain.Run,
	maxReviewRounds int,
	phase documentPhase,
) (domain.Run, error) {
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
	for round := 1; round <= maxReviewRounds; round++ {
		setDocumentPhaseFields(&run, phase.kind, round, "", "")
		if err := s.transition(store, &run, phase.producerState, phase.startEvent); err != nil {
			return run, err
		}

		producerJobID := fmt.Sprintf("%s-r%02d-designer", phase.kind, round)
		producerResult, jobErr := s.executeAgentJob(ctx, providers.Job{
			ID:       producerJobID,
			Provider: run.Roles[domain.RoleDesigner].Provider,
			Role:     domain.RoleDesigner,
			Project:  run.Project,
			JobDir: filepath.Join(
				store.RunDir(run.ID),
				"jobs",
				producerJobID,
			),
			Prompt: []byte(buildDocumentPrompt(
				designerInstructions,
				phase.kind,
				phase.input,
				previousArtifact,
				previousReview,
				round,
			)),
			Schema: phase.schema,
		})
		if jobErr != nil {
			return s.failRun(store, run, jobErr)
		}
		artifactBytes := withTrailingNewline(producerResult.StructuredOutput)
		artifactPath, err := store.WriteRunFile(
			run.ID,
			fmt.Sprintf("artifacts/%s/v%03d.json", phase.kind, round),
			artifactBytes,
		)
		if err != nil {
			return s.failRun(store, run, err)
		}
		setDocumentPhaseFields(&run, phase.kind, round, artifactPath, "")
		if err := s.transition(store, &run, phase.reviewState, phase.producedEvent); err != nil {
			return run, err
		}

		reviewerJobID := fmt.Sprintf("%s-r%02d-reviewer", phase.kind, round)
		subjectHash := sha256Hex(artifactBytes)
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
			Prompt: []byte(buildDocumentReviewPrompt(
				reviewerInstructions,
				phase.kind,
				phase.input,
				producerResult.StructuredOutput,
				round,
				run.ID,
				reviewerJobID,
				artifactPath,
				subjectHash,
			)),
			Schema: phase.reviewSchema,
		})
		if jobErr != nil {
			return s.failRun(store, run, jobErr)
		}
		reviewPath, err := store.WriteRunFile(
			run.ID,
			fmt.Sprintf("reviews/%s-v%03d.json", phase.kind, round),
			reviewerResult.StructuredOutput,
		)
		if err != nil {
			return s.failRun(store, run, err)
		}
		decision, err := domain.ParseReviewDecision(reviewerResult.StructuredOutput)
		if err != nil {
			return s.failRun(store, run, err)
		}
		if err := evaluateReviewGate(decision, reviewGateExpectation{
			RunID:       run.ID,
			JobID:       reviewerJobID,
			ReviewType:  phase.kind,
			SubjectPath: artifactPath,
			SubjectHash: subjectHash,
			MediaType:   "application/json",
		}); err != nil {
			return s.failRun(store, run, err)
		}
		setDocumentPhaseFields(&run, phase.kind, round, artifactPath, reviewPath)
		run.LastVerdict = decision.Verdict

		switch decision.Verdict {
		case domain.VerdictApproved:
			if err := s.transition(store, &run, phase.approvedState, phase.approvedEvent); err != nil {
				return run, err
			}
			return run, nil
		case domain.VerdictBlocked:
			run.WaitingReason = phase.kind + " review blocked"
			if err := s.transition(store, &run, domain.StateBlocked, phase.blockedEvent); err != nil {
				return run, err
			}
			return run, nil
		case domain.VerdictChangesRequested:
			previousArtifact = producerResult.StructuredOutput
			previousReview = reviewerResult.StructuredOutput
			if round == maxReviewRounds {
				run.WaitingReason = phase.kind + " review limit reached"
				if err := s.transition(store, &run, domain.StateWaitingForHuman, phase.limitEvent); err != nil {
					return run, err
				}
				return run, nil
			}
		default:
			return s.failRun(store, run, fmt.Errorf("unsupported review verdict %q", decision.Verdict))
		}
	}
	return s.failRun(store, run, fmt.Errorf("%s loop ended without a terminal state", phase.kind))
}

func buildDocumentPrompt(
	instructions, kind string,
	input, previousArtifact, previousReview []byte,
	round int,
) string {
	var prompt strings.Builder
	fmt.Fprintf(&prompt, "%s\n\n# Job\n\nArtifact: %s\nReview round: %d\n", instructions, kind, round)
	prompt.WriteString("\n## Approved inputs\n\n")
	prompt.Write(input)
	if len(previousArtifact) > 0 {
		fmt.Fprintf(&prompt, "\n\n## Previous %s artifact\n\n```json\n%s\n```\n", kind, prettyJSON(previousArtifact))
	}
	if len(previousReview) > 0 {
		fmt.Fprintf(&prompt, "\n\n## Required review feedback\n\n```json\n%s\n```\n", prettyJSON(previousReview))
	}
	fmt.Fprintf(
		&prompt,
		"\n\nProduce the %s artifact now. Return only the JSON object required by the supplied schema.",
		kind,
	)
	return prompt.String()
}

func buildDocumentReviewPrompt(
	instructions, kind string,
	input, artifact []byte,
	round int,
	runID, jobID, subjectPath, subjectHash string,
) string {
	return fmt.Sprintf(
		"%s\n\n# Job\n\nRun ID: %s\nJob ID: %s\nReview type: %s\nReview round: %d\n"+
			"Subject path: %s\nSubject SHA-256: %s\nSubject media type: application/json\n\n"+
			"## Approved inputs\n\n%s\n\n## %s artifact under review\n\n```json\n%s\n```\n\n"+
			"Copy the identifiers and subject fields exactly into the review. Review independently and return only the JSON object required by the supplied schema.",
		instructions,
		runID,
		jobID,
		kind,
		round,
		subjectPath,
		subjectHash,
		input,
		kind,
		prettyJSON(artifact),
	)
}

func joinPlanningInputs(parts ...any) []byte {
	var result strings.Builder
	for index := 0; index+1 < len(parts); index += 2 {
		label, _ := parts[index].(string)
		data, _ := parts[index+1].([]byte)
		fmt.Fprintf(&result, "## %s\n\n```json\n%s\n```\n\n", label, prettyJSON(data))
	}
	return []byte(strings.TrimSpace(result.String()))
}

func setDocumentPhaseFields(
	run *domain.Run,
	kind string,
	round int,
	artifactPath, reviewPath string,
) {
	switch kind {
	case "architecture":
		run.ArchitectureRound = round
		if artifactPath != "" {
			run.ArchitecturePath = artifactPath
		}
		if reviewPath != "" {
			run.ArchitectureReview = reviewPath
		}
	case "plan":
		run.PlanRound = round
		if artifactPath != "" {
			run.PlanPath = artifactPath
		}
		if reviewPath != "" {
			run.PlanReview = reviewPath
		}
	}
}
