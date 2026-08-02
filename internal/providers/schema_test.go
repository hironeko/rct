package providers

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hironeko/rct/schemas"
)

func TestEmbeddedSchemasCompileAndRejectIncompleteOutput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		load func() ([]byte, error)
	}{
		{name: "requirements", load: schemas.Requirements},
		{name: "architecture", load: schemas.Architecture},
		{name: "plan", load: schemas.Plan},
		{name: "implementation", load: schemas.Implementation},
		{name: "review", load: schemas.Review},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			data, err := test.load()
			if err != nil {
				t.Fatalf("load schema: %v", err)
			}
			schema, err := compileOutputSchema(data)
			if err != nil {
				t.Fatalf("compileOutputSchema() error: %v", err)
			}
			err = validateStructuredOutput(schema, []byte(`{"schema_version":"1.0"}`))
			if err == nil || !strings.Contains(err.Error(), "validate output") {
				t.Fatalf("validateStructuredOutput() error = %v, want required-field failure", err)
			}
		})
	}
}

func TestEmbeddedSchemasAvoidUnsupportedClaudeTopLevelCombinators(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		load func() ([]byte, error)
	}{
		{name: "requirements", load: schemas.Requirements},
		{name: "architecture", load: schemas.Architecture},
		{name: "plan", load: schemas.Plan},
		{name: "implementation", load: schemas.Implementation},
		{name: "review", load: schemas.Review},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			data, err := test.load()
			if err != nil {
				t.Fatalf("load schema: %v", err)
			}
			var root map[string]json.RawMessage
			if err := json.Unmarshal(data, &root); err != nil {
				t.Fatalf("decode schema: %v", err)
			}
			for _, keyword := range []string{"oneOf", "allOf", "anyOf"} {
				if _, exists := root[keyword]; exists {
					t.Fatalf("schema contains Claude-incompatible top-level %q", keyword)
				}
			}
		})
	}
}

func TestReviewSchemaSupportsWorkflowReviewTypesAndMediaTypes(t *testing.T) {
	t.Parallel()

	data, err := schemas.Review()
	if err != nil {
		t.Fatalf("load review schema: %v", err)
	}
	schema, err := compileOutputSchema(data)
	if err != nil {
		t.Fatalf("compileOutputSchema() error: %v", err)
	}

	for _, reviewType := range []string{"requirements", "architecture", "plan", "code", "final"} {
		reviewType := reviewType
		t.Run(reviewType, func(t *testing.T) {
			t.Parallel()
			output := validReviewOutput("approved")
			output["review_type"] = reviewType
			encoded, marshalErr := json.Marshal(output)
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			if validationErr := validateStructuredOutput(schema, encoded); validationErr != nil {
				t.Fatalf("validateStructuredOutput() error: %v", validationErr)
			}
		})
	}

	tests := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{
			name: "unsupported review type",
			mutate: func(output map[string]any) {
				output["review_type"] = "security"
			},
		},
		{
			name: "missing media type",
			mutate: func(output map[string]any) {
				delete(output["subject"].(map[string]string), "media_type")
			},
		},
		{
			name: "unsupported media type",
			mutate: func(output map[string]any) {
				output["subject"].(map[string]string)["media_type"] = "text/html"
			},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			output := validReviewOutput("approved")
			test.mutate(output)
			encoded, marshalErr := json.Marshal(output)
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			if validationErr := validateStructuredOutput(schema, encoded); validationErr == nil {
				t.Fatal("validateStructuredOutput() succeeded, want identity schema failure")
			}
		})
	}
}

func validReviewOutput(verdict string) map[string]any {
	return map[string]any{
		"schema_version": "1.0",
		"run_id":         "run-1",
		"job_id":         "job-1",
		"review_type":    "requirements",
		"subject": map[string]string{
			"path":       "artifacts/requirements/v001.json",
			"sha256":     strings.Repeat("a", 64),
			"media_type": "application/json",
		},
		"verdict": verdict,
		"summary": "Review summary",
		"scores": map[string]int{
			"clarity":      4,
			"completeness": 4,
			"feasibility":  4,
			"testability":  4,
			"risk_control": 4,
		},
		"required_changes":     []any{},
		"optional_suggestions": []string{},
		"open_questions":       []string{},
	}
}
