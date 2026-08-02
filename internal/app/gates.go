package app

import (
	"fmt"

	"github.com/hironeko/loop-engine/internal/domain"
)

// reviewGateExpectation identifies the exact immutable artifact review that is
// allowed to affect the current workflow state. A reviewer verdict is only one
// input to this deterministic gate.
type reviewGateExpectation struct {
	RunID       string
	JobID       string
	ReviewType  string
	SubjectPath string
	SubjectHash string
	MediaType   string
}

func evaluateReviewGate(
	decision domain.ReviewDecision,
	expected reviewGateExpectation,
) error {
	if decision.RunID != expected.RunID {
		return fmt.Errorf(
			"review run_id %q does not match %q",
			decision.RunID,
			expected.RunID,
		)
	}
	if decision.JobID != expected.JobID {
		return fmt.Errorf(
			"review job_id %q does not match %q",
			decision.JobID,
			expected.JobID,
		)
	}
	if decision.ReviewType != expected.ReviewType {
		return fmt.Errorf(
			"review_type %q does not match %q",
			decision.ReviewType,
			expected.ReviewType,
		)
	}
	if decision.Subject.Path != expected.SubjectPath {
		return fmt.Errorf(
			"review subject path %q does not match %q",
			decision.Subject.Path,
			expected.SubjectPath,
		)
	}
	if decision.Subject.SHA256 != expected.SubjectHash {
		return fmt.Errorf(
			"review subject sha256 %q does not match current artifact %q",
			decision.Subject.SHA256,
			expected.SubjectHash,
		)
	}
	if decision.Subject.MediaType != expected.MediaType {
		return fmt.Errorf(
			"review subject media_type %q does not match %q",
			decision.Subject.MediaType,
			expected.MediaType,
		)
	}
	return nil
}
