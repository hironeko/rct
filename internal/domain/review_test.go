package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseReviewDecisionEnforcesVerdictSemantics(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		verdict         Verdict
		requiredChanges []ReviewFinding
		openQuestions   []string
		wantError       string
	}{
		{
			name:    "approved",
			verdict: VerdictApproved,
		},
		{
			name:            "approved with required change",
			verdict:         VerdictApproved,
			requiredChanges: []ReviewFinding{{ID: "RC-001"}},
			wantError:       "approved review contains 1 required changes",
		},
		{
			name:          "approved with open question",
			verdict:       VerdictApproved,
			openQuestions: []string{"Which policy applies?"},
			wantError:     "approved review contains 1 open questions",
		},
		{
			name:            "changes requested",
			verdict:         VerdictChangesRequested,
			requiredChanges: []ReviewFinding{{ID: "RC-001"}},
		},
		{
			name:      "changes requested without required change",
			verdict:   VerdictChangesRequested,
			wantError: "changes_requested review must contain a required change",
		},
		{
			name:            "changes requested with open question",
			verdict:         VerdictChangesRequested,
			requiredChanges: []ReviewFinding{{ID: "RC-001"}},
			openQuestions:   []string{"Which policy applies?"},
			wantError:       "changes_requested review contains 1 open questions; use blocked",
		},
		{
			name:          "blocked",
			verdict:       VerdictBlocked,
			openQuestions: []string{"Which policy applies?"},
		},
		{
			name:      "blocked without open question",
			verdict:   VerdictBlocked,
			wantError: "blocked review must contain an open question",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			data, err := json.Marshal(ReviewDecision{
				SchemaVersion:   "1.0",
				Verdict:         test.verdict,
				RequiredChanges: test.requiredChanges,
				OpenQuestions:   test.openQuestions,
			})
			if err != nil {
				t.Fatal(err)
			}

			_, err = ParseReviewDecision(data)
			if test.wantError == "" {
				if err != nil {
					t.Fatalf("ParseReviewDecision() error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("ParseReviewDecision() error = %v, want %q", err, test.wantError)
			}
		})
	}
}
