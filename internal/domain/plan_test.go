package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseImplementationPlanRejectsShellVerification(t *testing.T) {
	t.Parallel()

	plan := validPlanDocument()
	plan["milestones"].([]any)[0].(map[string]any)["verification_commands"] = []any{
		map[string]any{"executable": "bash", "args": []string{"-c", "go test ./..."}},
	}
	data, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	_, err = ParseImplementationPlan(data)
	if err == nil || !strings.Contains(err.Error(), "is not allowed") {
		t.Fatalf("ParseImplementationPlan() error = %v, want shell rejection", err)
	}
}

func TestParseImplementationPlanRejectsForwardDependency(t *testing.T) {
	t.Parallel()

	plan := validPlanDocument()
	first := plan["milestones"].([]any)[0].(map[string]any)
	first["dependencies"] = []string{"M02"}
	second := map[string]any{}
	for key, value := range first {
		second[key] = value
	}
	second["id"] = "M02"
	second["dependencies"] = []string{}
	plan["milestones"] = []any{first, second}
	data, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	_, err = ParseImplementationPlan(data)
	if err == nil || !strings.Contains(err.Error(), "must appear earlier") {
		t.Fatalf("ParseImplementationPlan() error = %v, want dependency order rejection", err)
	}
}

func validPlanDocument() map[string]any {
	return map[string]any{
		"schema_version": "1.0",
		"title":          "Plan",
		"summary":        "Summary",
		"milestones": []any{map[string]any{
			"id": "M01", "objective": "Implement", "scope": []string{"Core"},
			"non_scope": []string{}, "dependencies": []string{},
			"change_areas":        []string{"internal"},
			"acceptance_criteria": []string{"Tests pass"},
			"verification_commands": []any{map[string]any{
				"executable": "go", "args": []string{"test", "./..."},
			}},
			"risks": []string{}, "done_when": []string{"Reviewed"},
		}},
	}
}
