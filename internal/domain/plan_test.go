package domain

import (
	"encoding/json"
	"os"
	"reflect"
	"sort"
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

func TestParseImplementationPlanRejectsExecutableOutsideAllowlist(t *testing.T) {
	t.Parallel()

	plan := validPlanDocument()
	plan["milestones"].([]any)[0].(map[string]any)["verification_commands"] = []any{
		map[string]any{"executable": "curl", "args": []string{"https://example.invalid"}},
	}
	data, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	_, err = ParseImplementationPlan(data)
	if err == nil || !strings.Contains(err.Error(), "is not allowed") {
		t.Fatalf("ParseImplementationPlan() error = %v, want allowlist rejection", err)
	}
}

func TestVerificationExecutableAllowlistMatchesPlanSchema(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../schemas/plan.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatal(err)
	}
	properties := schema["properties"].(map[string]any)
	milestones := properties["milestones"].(map[string]any)
	milestoneProperties := milestones["items"].(map[string]any)["properties"].(map[string]any)
	commands := milestoneProperties["verification_commands"].(map[string]any)
	commandProperties := commands["items"].(map[string]any)["properties"].(map[string]any)
	executable := commandProperties["executable"].(map[string]any)

	var schemaAllowlist []string
	for _, value := range executable["enum"].([]any) {
		schemaAllowlist = append(schemaAllowlist, value.(string))
	}
	var domainAllowlist []string
	for value := range allowedVerificationExecutables {
		domainAllowlist = append(domainAllowlist, value)
	}
	sort.Strings(schemaAllowlist)
	sort.Strings(domainAllowlist)
	if !reflect.DeepEqual(schemaAllowlist, domainAllowlist) {
		t.Fatalf("schema allowlist = %v, domain allowlist = %v", schemaAllowlist, domainAllowlist)
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
