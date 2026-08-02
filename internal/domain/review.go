package domain

import (
	"encoding/json"
	"fmt"
)

type Verdict string

const (
	VerdictApproved         Verdict = "approved"
	VerdictChangesRequested Verdict = "changes_requested"
	VerdictBlocked          Verdict = "blocked"
)

type ReviewDecision struct {
	SchemaVersion       string          `json:"schema_version"`
	RunID               string          `json:"run_id"`
	JobID               string          `json:"job_id"`
	ReviewType          string          `json:"review_type"`
	Subject             ReviewSubject   `json:"subject"`
	Verdict             Verdict         `json:"verdict"`
	Summary             string          `json:"summary"`
	Scores              map[string]int  `json:"scores"`
	RequiredChanges     []ReviewFinding `json:"required_changes"`
	OptionalSuggestions []string        `json:"optional_suggestions"`
	OpenQuestions       []string        `json:"open_questions"`
}

type ReviewSubject struct {
	Path      string `json:"path"`
	SHA256    string `json:"sha256"`
	MediaType string `json:"media_type"`
}

type ReviewFinding struct {
	ID              string `json:"id"`
	Severity        string `json:"severity"`
	Target          string `json:"target"`
	Problem         string `json:"problem"`
	Rationale       string `json:"rationale"`
	ExpectedOutcome string `json:"expected_outcome"`
}

func ParseReviewDecision(data []byte) (ReviewDecision, error) {
	var decision ReviewDecision
	if err := json.Unmarshal(data, &decision); err != nil {
		return ReviewDecision{}, fmt.Errorf("decode review decision: %w", err)
	}
	switch decision.Verdict {
	case VerdictApproved:
		if len(decision.RequiredChanges) != 0 {
			return ReviewDecision{}, fmt.Errorf(
				"approved review contains %d required changes",
				len(decision.RequiredChanges),
			)
		}
		if len(decision.OpenQuestions) != 0 {
			return ReviewDecision{}, fmt.Errorf(
				"approved review contains %d open questions",
				len(decision.OpenQuestions),
			)
		}
	case VerdictChangesRequested:
		if len(decision.RequiredChanges) == 0 {
			return ReviewDecision{}, fmt.Errorf(
				"changes_requested review must contain a required change",
			)
		}
		if len(decision.OpenQuestions) != 0 {
			return ReviewDecision{}, fmt.Errorf(
				"changes_requested review contains %d open questions; use blocked",
				len(decision.OpenQuestions),
			)
		}
	case VerdictBlocked:
		if len(decision.OpenQuestions) == 0 {
			return ReviewDecision{}, fmt.Errorf(
				"blocked review must contain an open question",
			)
		}
	default:
		return ReviewDecision{}, fmt.Errorf("unsupported review verdict %q", decision.Verdict)
	}
	return decision, nil
}
