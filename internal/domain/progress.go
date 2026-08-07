package domain

import "time"

const ProgressSchemaVersion = "progress-v1"

type ActivityStatus string

const (
	ActivityQueued    ActivityStatus = "queued"
	ActivityRunning   ActivityStatus = "running"
	ActivityWaiting   ActivityStatus = "waiting"
	ActivityCompleted ActivityStatus = "completed"
	ActivityFailed    ActivityStatus = "failed"
	ActivityCancelled ActivityStatus = "cancelled"
	ActivityStale     ActivityStatus = "stale"
)

type SafeProgressError struct {
	Code       string `json:"code"`
	Summary    string `json:"summary"`
	Retryable  bool   `json:"retryable"`
	NextAction string `json:"next_action,omitempty"`
}

type CurrentActivity struct {
	SchemaVersion       string             `json:"schema_version"`
	RunID               string             `json:"run_id"`
	Revision            uint64             `json:"revision"`
	Status              ActivityStatus     `json:"status"`
	Phase               string             `json:"phase"`
	Action              string             `json:"action"`
	Role                string             `json:"role,omitempty"`
	Provider            string             `json:"provider,omitempty"`
	Backend             string             `json:"backend"`
	JobID               string             `json:"job_id,omitempty"`
	Round               int                `json:"round,omitempty"`
	MaxRounds           int                `json:"max_rounds,omitempty"`
	ArtifactKind        string             `json:"artifact_kind,omitempty"`
	CandidateVersion    int                `json:"candidate_version,omitempty"`
	PreviousVerdict     string             `json:"previous_verdict,omitempty"`
	RequiredChangeCount int                `json:"required_change_count,omitempty"`
	StartedAt           time.Time          `json:"started_at"`
	LastHeartbeatAt     time.Time          `json:"last_heartbeat_at"`
	WaitingReason       string             `json:"waiting_reason,omitempty"`
	Error               *SafeProgressError `json:"error,omitempty"`
}

type ProgressEvent struct {
	SchemaVersion string         `json:"schema_version,omitempty"`
	Sequence      uint64         `json:"seq"`
	Timestamp     time.Time      `json:"timestamp"`
	RunID         string         `json:"run_id"`
	Type          string         `json:"type"`
	StateBefore   string         `json:"state_before,omitempty"`
	StateAfter    string         `json:"state_after,omitempty"`
	Phase         string         `json:"phase,omitempty"`
	Role          string         `json:"role,omitempty"`
	Provider      string         `json:"provider,omitempty"`
	Backend       string         `json:"backend,omitempty"`
	JobID         string         `json:"job_id,omitempty"`
	Round         int            `json:"round,omitempty"`
	ArtifactKind  string         `json:"artifact_kind,omitempty"`
	Version       int            `json:"version,omitempty"`
	Summary       string         `json:"summary,omitempty"`
	Data          map[string]any `json:"data,omitempty"`
}

type PhaseProgress struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Status string `json:"status"`
}

type Gauge struct {
	Kind        string `json:"kind"`
	Revision    uint64 `json:"revision"`
	Completed   int    `json:"completed"`
	Total       int    `json:"total"`
	Label       string `json:"label"`
	Invalidated bool   `json:"invalidated,omitempty"`
	Reason      string `json:"reason,omitempty"`
}

type ProgressSnapshot struct {
	SchemaVersion string           `json:"schema_version"`
	RunID         string           `json:"run_id"`
	Project       string           `json:"project"`
	Backend       string           `json:"backend"`
	Mode          RunMode          `json:"mode"`
	State         WorkflowState    `json:"state"`
	StateRevision uint64           `json:"state_revision"`
	Activity      *CurrentActivity `json:"activity,omitempty"`
	Phases        []PhaseProgress  `json:"phases"`
	Gauges        []Gauge          `json:"gauges"`
	LastEventSeq  uint64           `json:"last_event_seq"`
	UpdatedAt     time.Time        `json:"updated_at"`
	NextAction    string           `json:"next_action,omitempty"`
}

func ProjectProgress(run Run, activity *CurrentActivity, lastEventSeq uint64) ProgressSnapshot {
	phaseIDs := macroPhaseIDs(run.Mode)
	completed := completedMacroPhases(run)
	currentPhase := phaseForState(run.State)
	if run.State == StateWaitingForHuman && run.Interruption != nil && run.Interruption.Phase != "" {
		currentPhase = run.Interruption.Phase
	}
	if currentPhase == "" && activity != nil {
		currentPhase = activity.Phase
	}
	phases := make([]PhaseProgress, 0, len(phaseIDs))
	for _, id := range phaseIDs {
		status := "not_started"
		if completed[id] {
			status = "completed"
		} else if currentPhase == id {
			status = "running"
			if run.State == StateAwaitingApproval || run.State == StateWaitingForHuman || run.State == StateBlocked {
				status = "waiting"
			}
			if run.State == StateFailed {
				status = "failed"
			}
		}
		phases = append(phases, PhaseProgress{ID: id, Label: phaseLabel(id), Status: status})
	}

	macro := Gauge{Kind: "macro_phases", Revision: run.Revision, Total: len(phaseIDs), Label: "phases complete"}
	for _, id := range phaseIDs {
		if completed[id] {
			macro.Completed++
		}
	}
	gauges := []Gauge{macro}
	if run.PlanPath != "" && (run.CurrentMilestoneID != "" || len(run.CompletedMilestones) > 0) {
		gauges = append(gauges, Gauge{
			Kind: "milestones", Revision: run.Revision, Completed: len(run.CompletedMilestones),
			Total: maxInt(run.CurrentMilestone, len(run.CompletedMilestones)), Label: "milestones approved",
		})
	}
	next := ""
	switch run.State {
	case StateAwaitingApproval:
		next = "Run rct approve --project <path>"
	case StateImplementationReady:
		next = "Run rct implement --project <path>"
	case StateWaitingForHuman:
		if run.Interruption != nil && run.Interruption.Code == "GIT_BOOTSTRAP_REQUIRED" {
			next = "Run rct init --project <path> --request-file <request.md> --yes, then rct resume --project <path>"
		} else if run.Interruption != nil {
			next = "Resolve the preflight issue, then run rct resume --project <path>"
		} else {
			next = "Inspect the waiting reason and run status before resuming"
		}
	case StateBlocked:
		next = "Inspect the waiting reason and run status"
	case StateFailed:
		next = "Inspect the local job directory and run rct status"
	}
	return ProgressSnapshot{
		SchemaVersion: ProgressSchemaVersion, RunID: run.ID, Project: run.Project, Backend: run.Backend,
		Mode: run.Mode, State: run.State, StateRevision: run.Revision, Activity: activity,
		Phases: phases, Gauges: gauges, LastEventSeq: lastEventSeq, UpdatedAt: run.UpdatedAt, NextAction: next,
	}
}

func macroPhaseIDs(mode RunMode) []string {
	base := []string{"requirements", "architecture", "plan"}
	if mode == ModeDesignOnly {
		return base
	}
	base = append(base, "implementation_preflight")
	if mode == ModeSupervised {
		base = append(base, "implementation_approval")
	}
	return append(base, "implementation", "final_verification", "final_review")
}

func completedMacroPhases(run Run) map[string]bool {
	result := map[string]bool{}
	state := run.State
	if run.RequirementsPath != "" && state != StateRequirementsDraft && state != StateRequirementsReview {
		result["requirements"] = true
	}
	if run.ArchitecturePath != "" && state != StateArchitectureDraft && state != StateArchitectureReview {
		result["architecture"] = true
	}
	if run.PlanPath != "" && state != StatePlanDraft && state != StatePlanReview {
		result["plan"] = true
	}
	if run.BaseCommit != "" {
		result["implementation_preflight"] = true
	}
	if run.Mode == ModeSupervised && run.BaseCommit != "" && run.Approval != nil && run.Approval.SubjectSHA256 == run.ApprovalTargetHash {
		result["implementation_approval"] = true
	}
	if state == StateFinalVerification || state == StateFinalReview || state == StateCompleted {
		result["implementation"] = true
	}
	if state == StateFinalReview || state == StateCompleted {
		result["final_verification"] = true
	}
	if state == StateCompleted {
		result["final_review"] = true
	}
	return result
}

func phaseForState(state WorkflowState) string {
	switch state {
	case StateRequirementsDraft, StateRequirementsReview, StateRequirementsApproved:
		return "requirements"
	case StateArchitectureDraft, StateArchitectureReview, StateArchitectureApproved:
		return "architecture"
	case StatePlanDraft, StatePlanReview, StatePlanApproved:
		return "plan"
	case StateImplementationPreflight:
		return "implementation_preflight"
	case StateAwaitingApproval:
		return "implementation_approval"
	case StateImplementationReady, StateMilestoneImplementation, StateMilestoneVerification, StateMilestoneReview, StateMilestoneFix, StateMilestoneApproved:
		return "implementation"
	case StateFinalVerification:
		return "final_verification"
	case StateFinalReview:
		return "final_review"
	case StateCompleted:
		return "completed"
	default:
		return ""
	}
}

func phaseLabel(id string) string {
	labels := map[string]string{
		"requirements": "Requirements", "architecture": "Architecture", "plan": "Implementation Plan",
		"implementation_preflight": "Implementation preflight", "implementation_approval": "Human approval",
		"implementation": "Implementation", "final_verification": "Final verification", "final_review": "Final review",
	}
	return labels[id]
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
