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
	StateIntake               WorkflowState = "INTAKE"
	StateRequirementsDraft    WorkflowState = "REQUIREMENTS_DRAFT"
	StateRequirementsReview   WorkflowState = "REQUIREMENTS_REVIEW"
	StateRequirementsApproved WorkflowState = "REQUIREMENTS_APPROVED"
	StateWaitingForHuman      WorkflowState = "WAITING_FOR_HUMAN"
	StateBlocked              WorkflowState = "BLOCKED"
	StateFailed               WorkflowState = "FAILED"
)

type RoleBinding struct {
	Role      Role     `json:"role"`
	Provider  Provider `json:"provider"`
	RoleID    string   `json:"role_id"`
	SessionID string   `json:"session_id"`
}

type Run struct {
	SchemaVersion      string               `json:"schema_version"`
	ID                 string               `json:"id"`
	Project            string               `json:"project"`
	Mode               RunMode              `json:"mode"`
	Backend            string               `json:"backend"`
	State              WorkflowState        `json:"state"`
	Roles              map[Role]RoleBinding `json:"roles"`
	RequestPath        string               `json:"request_path"`
	RequirementsRound  int                  `json:"requirements_round"`
	MaxReviewRounds    int                  `json:"max_review_rounds"`
	RequirementsPath   string               `json:"requirements_path,omitempty"`
	RequirementsReview string               `json:"requirements_review_path,omitempty"`
	LastVerdict        Verdict              `json:"last_verdict,omitempty"`
	Failure            string               `json:"failure,omitempty"`
	CreatedAt          time.Time            `json:"created_at"`
	UpdatedAt          time.Time            `json:"updated_at"`
	Revision           uint64               `json:"revision"`
}
