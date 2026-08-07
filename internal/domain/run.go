package domain

import (
	"fmt"
	"strings"
	"time"
)

type RunMode string

const (
	ModeSupervised RunMode = "supervised"
	ModeAutonomous RunMode = "autonomous"
	ModeDesignOnly RunMode = "design-only"
)

func ParseRunMode(value string) (RunMode, error) {
	switch RunMode(strings.ToLower(strings.TrimSpace(value))) {
	case ModeSupervised:
		return ModeSupervised, nil
	case ModeAutonomous:
		return ModeAutonomous, nil
	case ModeDesignOnly:
		return ModeDesignOnly, nil
	default:
		return "", fmt.Errorf(
			"unsupported mode %q: expected supervised, autonomous, or design-only",
			value,
		)
	}
}

type WorkflowState string

const (
	StateIntake                  WorkflowState = "INTAKE"
	StateRequirementsDraft       WorkflowState = "REQUIREMENTS_DRAFT"
	StateRequirementsReview      WorkflowState = "REQUIREMENTS_REVIEW"
	StateRequirementsApproved    WorkflowState = "REQUIREMENTS_APPROVED"
	StateArchitectureDraft       WorkflowState = "ARCHITECTURE_DRAFT"
	StateArchitectureReview      WorkflowState = "ARCHITECTURE_REVIEW"
	StateArchitectureApproved    WorkflowState = "ARCHITECTURE_APPROVED"
	StatePlanDraft               WorkflowState = "PLAN_DRAFT"
	StatePlanReview              WorkflowState = "PLAN_REVIEW"
	StatePlanApproved            WorkflowState = "PLAN_APPROVED"
	StateImplementationPreflight WorkflowState = "IMPLEMENTATION_PREFLIGHT"
	StateAwaitingApproval        WorkflowState = "AWAITING_IMPLEMENTATION_APPROVAL"
	StateImplementationReady     WorkflowState = "IMPLEMENTATION_READY"
	StateMilestoneImplementation WorkflowState = "MILESTONE_IMPLEMENTATION"
	StateMilestoneVerification   WorkflowState = "MILESTONE_VERIFICATION"
	StateMilestoneReview         WorkflowState = "MILESTONE_REVIEW"
	StateMilestoneFix            WorkflowState = "MILESTONE_FIX"
	StateMilestoneApproved       WorkflowState = "MILESTONE_APPROVED"
	StateFinalVerification       WorkflowState = "FINAL_VERIFICATION"
	StateFinalReview             WorkflowState = "FINAL_REVIEW"
	StateCompleted               WorkflowState = "COMPLETED"
	StateWaitingForHuman         WorkflowState = "WAITING_FOR_HUMAN"
	StateBlocked                 WorkflowState = "BLOCKED"
	StateFailed                  WorkflowState = "FAILED"
)

type RoleBinding struct {
	Role      Role     `json:"role"`
	Provider  Provider `json:"provider"`
	RoleID    string   `json:"role_id"`
	SessionID string   `json:"session_id"`
}

type Run struct {
	SchemaVersion        string                 `json:"schema_version"`
	EventProtocolVersion string                 `json:"event_protocol_version,omitempty"`
	ID                   string                 `json:"id"`
	Project              string                 `json:"project"`
	Mode                 RunMode                `json:"mode"`
	Backend              string                 `json:"backend"`
	State                WorkflowState          `json:"state"`
	Roles                map[Role]RoleBinding   `json:"roles"`
	RequestPath          string                 `json:"request_path"`
	RequirementsRound    int                    `json:"requirements_round"`
	MaxReviewRounds      int                    `json:"max_review_rounds"`
	RequirementsPath     string                 `json:"requirements_path,omitempty"`
	RequirementsReview   string                 `json:"requirements_review_path,omitempty"`
	ArchitectureRound    int                    `json:"architecture_round,omitempty"`
	ArchitecturePath     string                 `json:"architecture_path,omitempty"`
	ArchitectureReview   string                 `json:"architecture_review_path,omitempty"`
	PlanRound            int                    `json:"plan_round,omitempty"`
	PlanPath             string                 `json:"plan_path,omitempty"`
	PlanReview           string                 `json:"plan_review_path,omitempty"`
	PlanSHA256           string                 `json:"plan_sha256,omitempty"`
	ApprovalTargetHash   string                 `json:"approval_target_sha256,omitempty"`
	Approval             *HumanApprovalRecord   `json:"human_approval,omitempty"`
	ApprovalPath         string                 `json:"human_approval_path,omitempty"`
	CurrentMilestone     int                    `json:"current_milestone,omitempty"`
	CurrentMilestoneID   string                 `json:"current_milestone_id,omitempty"`
	ImplementationRound  int                    `json:"implementation_round,omitempty"`
	VerificationAttempts int                    `json:"verification_attempts,omitempty"`
	ImplementationPath   string                 `json:"implementation_path,omitempty"`
	VerificationPath     string                 `json:"verification_path,omitempty"`
	CodeReviewPath       string                 `json:"code_review_path,omitempty"`
	CompletedMilestones  []string               `json:"completed_milestones,omitempty"`
	RepositoryRoot       string                 `json:"repository_root,omitempty"`
	ProjectRelative      string                 `json:"project_relative,omitempty"`
	BaseCommit           string                 `json:"implementation_base_commit,omitempty"`
	PreflightCheckedAt   time.Time              `json:"implementation_preflight_checked_at,omitempty"`
	Interruption         *PreflightInterruption `json:"preflight_interruption,omitempty"`
	WaitingReason        string                 `json:"waiting_reason,omitempty"`
	LastVerdict          Verdict                `json:"last_verdict,omitempty"`
	Failure              string                 `json:"failure,omitempty"`
	CreatedAt            time.Time              `json:"created_at"`
	UpdatedAt            time.Time              `json:"updated_at"`
	Revision             uint64                 `json:"revision"`
}

type HumanApprovalRecord struct {
	SchemaVersion  string    `json:"schema_version"`
	ID             string    `json:"id"`
	RunID          string    `json:"run_id"`
	GateKind       string    `json:"gate_kind"`
	Phase          string    `json:"phase"`
	SubjectPath    string    `json:"subject_path"`
	SubjectSHA256  string    `json:"subject_sha256"`
	BaselineCommit string    `json:"baseline_commit"`
	Approver       string    `json:"approver"`
	Note           string    `json:"note,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	ConsumedAt     time.Time `json:"consumed_at"`
	StateRevision  uint64    `json:"state_revision"`
}

type PreflightInterruption struct {
	Code             string        `json:"code"`
	Phase            string        `json:"phase"`
	ResumeState      WorkflowState `json:"resume_state"`
	DetectedRevision uint64        `json:"detected_revision"`
	PlanSHA256       string        `json:"plan_sha256"`
	BaselineCommit   string        `json:"baseline_commit,omitempty"`
	Remediation      []string      `json:"remediation"`
	CreatedAt        time.Time     `json:"created_at"`
}
