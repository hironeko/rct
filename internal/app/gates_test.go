package app

import (
	"strings"
	"testing"

	"github.com/hironeko/loop-engine/internal/domain"
)

func TestEvaluateReviewGateRequiresExactReviewIdentity(t *testing.T) {
	t.Parallel()

	expected := reviewGateExpectation{
		RunID:       "run-1",
		JobID:       "job-1",
		ReviewType:  "requirements",
		SubjectPath: "artifacts/requirements/v001.md",
		SubjectHash: strings.Repeat("a", 64),
		MediaType:   "text/markdown",
	}
	valid := domain.ReviewDecision{
		RunID:      expected.RunID,
		JobID:      expected.JobID,
		ReviewType: expected.ReviewType,
		Subject: domain.ReviewSubject{
			Path:      expected.SubjectPath,
			SHA256:    expected.SubjectHash,
			MediaType: expected.MediaType,
		},
		Verdict: domain.VerdictApproved,
	}
	if err := evaluateReviewGate(valid, expected); err != nil {
		t.Fatalf("evaluateReviewGate() error: %v", err)
	}

	tests := []struct {
		name      string
		mutate    func(*domain.ReviewDecision)
		wantError string
	}{
		{
			name: "run id",
			mutate: func(decision *domain.ReviewDecision) {
				decision.RunID = "stale-run"
			},
			wantError: "review run_id",
		},
		{
			name: "job id",
			mutate: func(decision *domain.ReviewDecision) {
				decision.JobID = "stale-job"
			},
			wantError: "review job_id",
		},
		{
			name: "review type",
			mutate: func(decision *domain.ReviewDecision) {
				decision.ReviewType = "plan"
			},
			wantError: "review_type",
		},
		{
			name: "subject path",
			mutate: func(decision *domain.ReviewDecision) {
				decision.Subject.Path = "artifacts/requirements/v000.md"
			},
			wantError: "review subject path",
		},
		{
			name: "subject hash",
			mutate: func(decision *domain.ReviewDecision) {
				decision.Subject.SHA256 = strings.Repeat("b", 64)
			},
			wantError: "review subject sha256",
		},
		{
			name: "subject media type",
			mutate: func(decision *domain.ReviewDecision) {
				decision.Subject.MediaType = "application/json"
			},
			wantError: "review subject media_type",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			decision := valid
			test.mutate(&decision)
			err := evaluateReviewGate(decision, expected)
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("evaluateReviewGate() error = %v, want %q", err, test.wantError)
			}
		})
	}
}
