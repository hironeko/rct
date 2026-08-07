package app

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/hironeko/rct/internal/domain"
	"github.com/hironeko/rct/internal/store/filesystem"
)

type ProgressQueryService struct{}

type PublicRunSnapshot struct {
	SchemaVersion string                 `json:"schema_version"`
	RunID         string                 `json:"run_id"`
	ProjectName   string                 `json:"project_name"`
	Backend       string                 `json:"backend"`
	Mode          domain.RunMode         `json:"mode"`
	State         domain.WorkflowState   `json:"state"`
	StateRevision uint64                 `json:"state_revision"`
	Roles         map[string]string      `json:"roles"`
	Activity      *PublicActivity        `json:"activity,omitempty"`
	Phases        []domain.PhaseProgress `json:"phases"`
	Gauges        []domain.Gauge         `json:"gauges"`
	Artifacts     map[string]string      `json:"artifacts,omitempty"`
	LastEventSeq  uint64                 `json:"last_event_seq"`
	UpdatedAt     time.Time              `json:"updated_at"`
	WaitingReason string                 `json:"waiting_reason,omitempty"`
	NextAction    string                 `json:"next_action,omitempty"`
}

type PublicActivity struct {
	Revision            uint64                    `json:"revision"`
	Status              domain.ActivityStatus     `json:"status"`
	Phase               string                    `json:"phase"`
	Action              string                    `json:"action"`
	Role                string                    `json:"role,omitempty"`
	Provider            string                    `json:"provider,omitempty"`
	Backend             string                    `json:"backend"`
	JobID               string                    `json:"job_id,omitempty"`
	Round               int                       `json:"round,omitempty"`
	MaxRounds           int                       `json:"max_rounds,omitempty"`
	ArtifactKind        string                    `json:"artifact_kind,omitempty"`
	CandidateVersion    int                       `json:"candidate_version,omitempty"`
	PreviousVerdict     string                    `json:"previous_verdict,omitempty"`
	RequiredChangeCount int                       `json:"required_change_count,omitempty"`
	StartedAt           time.Time                 `json:"started_at"`
	LastHeartbeatAt     time.Time                 `json:"last_heartbeat_at"`
	WaitingReason       string                    `json:"waiting_reason,omitempty"`
	Error               *domain.SafeProgressError `json:"error,omitempty"`
}

type PublicProgressEvent struct {
	SchemaVersion string    `json:"schema_version,omitempty"`
	Sequence      uint64    `json:"sequence"`
	Timestamp     time.Time `json:"timestamp"`
	RunID         string    `json:"run_id"`
	Type          string    `json:"type"`
	StateBefore   string    `json:"state_before,omitempty"`
	StateAfter    string    `json:"state_after,omitempty"`
	Phase         string    `json:"phase,omitempty"`
	Role          string    `json:"role,omitempty"`
	Provider      string    `json:"provider,omitempty"`
	Backend       string    `json:"backend,omitempty"`
	JobID         string    `json:"job_id,omitempty"`
	Round         int       `json:"round,omitempty"`
	ArtifactKind  string    `json:"artifact_kind,omitempty"`
	Version       int       `json:"version,omitempty"`
}

func (ProgressQueryService) Snapshot(project, runID string) (PublicRunSnapshot, error) {
	store := filesystem.New(project)
	run, err := store.Load(runID)
	if err != nil {
		return PublicRunSnapshot{}, err
	}
	snapshot, err := store.Progress(runID)
	if err != nil {
		return PublicRunSnapshot{}, err
	}
	result := PublicRunSnapshot{
		SchemaVersion: domain.ProgressSchemaVersion,
		RunID:         run.ID,
		ProjectName:   filepath.Base(run.Project),
		Backend:       run.Backend,
		Mode:          run.Mode,
		State:         run.State,
		StateRevision: run.Revision,
		Roles:         publicRoles(run.Roles),
		Activity:      publicActivity(snapshot.Activity, run.State),
		Phases:        snapshot.Phases,
		Gauges:        snapshot.Gauges,
		Artifacts:     publicArtifacts(run),
		LastEventSeq:  snapshot.LastEventSeq,
		UpdatedAt:     snapshot.UpdatedAt,
		WaitingReason: publicWaitingReason(run.State),
		NextAction:    snapshot.NextAction,
	}
	return result, nil
}

func (ProgressQueryService) Events(project, runID string, after uint64, limit int) ([]PublicProgressEvent, uint64, error) {
	if limit <= 0 || limit > 256 {
		limit = 100
	}
	events, err := filesystem.New(project).ReadEvents(runID, after)
	if err != nil {
		return nil, 0, err
	}
	if len(events) > limit {
		events = events[:limit]
	}
	result := make([]PublicProgressEvent, 0, len(events))
	for _, event := range events {
		result = append(result, PublicProgressEvent{
			SchemaVersion: event.SchemaVersion,
			Sequence:      event.Sequence,
			Timestamp:     event.Timestamp,
			RunID:         event.RunID,
			Type:          event.Type,
			StateBefore:   event.StateBefore,
			StateAfter:    event.StateAfter,
			Phase:         event.Phase,
			Role:          event.Role,
			Provider:      event.Provider,
			Backend:       event.Backend,
			JobID:         event.JobID,
			Round:         event.Round,
			ArtifactKind:  event.ArtifactKind,
			Version:       event.Version,
		})
	}
	var next uint64 = after
	if len(result) > 0 {
		next = result[len(result)-1].Sequence
	}
	return result, next, nil
}

func (q ProgressQueryService) Activity(project, runID string) (*PublicActivity, error) {
	snapshot, err := q.Snapshot(project, runID)
	if err != nil {
		return nil, err
	}
	return snapshot.Activity, nil
}

func publicRoles(bindings map[domain.Role]domain.RoleBinding) map[string]string {
	roles := make(map[string]string, len(bindings))
	for role, binding := range bindings {
		roles[string(role)] = string(binding.Provider)
	}
	return roles
}

func publicActivity(activity *domain.CurrentActivity, state domain.WorkflowState) *PublicActivity {
	if activity == nil {
		return nil
	}
	previousVerdict := activity.PreviousVerdict
	if previousVerdict != string(domain.VerdictApproved) && previousVerdict != string(domain.VerdictChangesRequested) && previousVerdict != string(domain.VerdictBlocked) {
		previousVerdict = ""
	}
	return &PublicActivity{
		Revision:            activity.Revision,
		Status:              activity.Status,
		Phase:               safeIdentifier(activity.Phase),
		Action:              safeIdentifier(activity.Action),
		Role:                safeIdentifier(activity.Role),
		Provider:            safeProvider(activity.Provider),
		Backend:             safeIdentifier(activity.Backend),
		JobID:               safeJobID(activity.JobID),
		Round:               activity.Round,
		MaxRounds:           activity.MaxRounds,
		ArtifactKind:        safeIdentifier(activity.ArtifactKind),
		CandidateVersion:    activity.CandidateVersion,
		PreviousVerdict:     previousVerdict,
		RequiredChangeCount: activity.RequiredChangeCount,
		StartedAt:           activity.StartedAt,
		LastHeartbeatAt:     activity.LastHeartbeatAt,
		WaitingReason:       publicWaitingReason(state),
		Error:               publicError(activity.Error),
	}
}

func publicError(source *domain.SafeProgressError) *domain.SafeProgressError {
	if source == nil {
		return nil
	}
	known := map[string]domain.SafeProgressError{
		"PROVIDER_JOB_FAILED":   {Code: "PROVIDER_JOB_FAILED", Summary: "The provider job did not produce a valid result", Retryable: true, NextAction: "Inspect the run from the local CLI"},
		"LOG_SINK_BACKPRESSURE": {Code: "LOG_SINK_BACKPRESSURE", Summary: "The job stopped because its diagnostic log could not be stored safely", Retryable: true, NextAction: "Inspect the run from the local CLI"},
		"VERIFICATION_FAILED":   {Code: "VERIFICATION_FAILED", Summary: "A verification command did not complete successfully", Retryable: true, NextAction: "Inspect verification from the local CLI"},
		"RUN_FAILED":            {Code: "RUN_FAILED", Summary: "The run stopped before completion", Retryable: true, NextAction: "Inspect the run from the local CLI"},
	}
	value, ok := known[source.Code]
	if !ok {
		value = domain.SafeProgressError{Code: "RUN_FAILED", Summary: "The run stopped before completion", Retryable: true, NextAction: "Inspect the run from the local CLI"}
	}
	return &value
}

func publicArtifacts(run domain.Run) map[string]string {
	candidates := map[string]string{
		"requirements":        run.RequirementsPath,
		"requirements_review": run.RequirementsReview,
		"architecture":        run.ArchitecturePath,
		"architecture_review": run.ArchitectureReview,
		"plan":                run.PlanPath,
		"plan_review":         run.PlanReview,
		"implementation":      run.ImplementationPath,
		"verification":        run.VerificationPath,
		"code_review":         run.CodeReviewPath,
	}
	result := map[string]string{}
	for kind, path := range candidates {
		if relative, ok := publicRunRelativePath(run.ID, path); ok {
			result[kind] = relative
		}
	}
	return result
}

func publicRunRelativePath(runID, value string) (string, bool) {
	if value == "" {
		return "", false
	}
	normalized := filepath.ToSlash(filepath.Clean(value))
	prefix := ".rct/runs/" + runID + "/"
	if !strings.HasPrefix(normalized, prefix) {
		return "", false
	}
	relative := strings.TrimPrefix(normalized, prefix)
	if relative == "" || relative == "." || strings.HasPrefix(relative, "../") {
		return "", false
	}
	return relative, true
}

func publicWaitingReason(state domain.WorkflowState) string {
	switch state {
	case domain.StateAwaitingApproval:
		return "Human implementation approval is required"
	case domain.StateWaitingForHuman:
		return "Human input is required before the run can continue"
	case domain.StateBlocked:
		return "The run is blocked and requires attention"
	case domain.StateFailed:
		return "The run stopped before completion"
	default:
		return ""
	}
}

func safeIdentifier(value string) string {
	if value == "" {
		return ""
	}
	for _, char := range value {
		if !((char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '_' || char == '-') {
			return "unknown"
		}
	}
	return value
}

func safeProvider(value string) string {
	if value == string(domain.ProviderCodex) || value == string(domain.ProviderClaude) || value == "" {
		return value
	}
	return "unknown"
}

func safeJobID(value string) string {
	if len(value) > 160 {
		return "unknown"
	}
	return safeIdentifier(value)
}

func ValidateEventCursor(after, highWater uint64) error {
	if after > highWater {
		return fmt.Errorf("resync required: event cursor %d is ahead of high-water mark %d", after, highWater)
	}
	return nil
}
