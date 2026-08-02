package domain

import (
	"encoding/json"
	"fmt"
	"strings"
)

type CommandSpec struct {
	Executable string   `json:"executable"`
	Args       []string `json:"args"`
}

type ImplementationPlan struct {
	SchemaVersion string      `json:"schema_version"`
	Title         string      `json:"title"`
	Summary       string      `json:"summary"`
	Milestones    []Milestone `json:"milestones"`
}

type Milestone struct {
	ID                   string        `json:"id"`
	Objective            string        `json:"objective"`
	Scope                []string      `json:"scope"`
	NonScope             []string      `json:"non_scope"`
	Dependencies         []string      `json:"dependencies"`
	ChangeAreas          []string      `json:"change_areas"`
	AcceptanceCriteria   []string      `json:"acceptance_criteria"`
	VerificationCommands []CommandSpec `json:"verification_commands"`
	Risks                []string      `json:"risks"`
	DoneWhen             []string      `json:"done_when"`
}

func ParseImplementationPlan(data []byte) (ImplementationPlan, error) {
	var plan ImplementationPlan
	if err := json.Unmarshal(data, &plan); err != nil {
		return ImplementationPlan{}, fmt.Errorf("decode implementation plan: %w", err)
	}
	if plan.SchemaVersion != "1.0" {
		return ImplementationPlan{}, fmt.Errorf("unsupported plan schema version %q", plan.SchemaVersion)
	}
	if len(plan.Milestones) == 0 {
		return ImplementationPlan{}, fmt.Errorf("implementation plan has no milestones")
	}
	seen := make(map[string]int, len(plan.Milestones))
	for index, milestone := range plan.Milestones {
		if strings.TrimSpace(milestone.ID) == "" {
			return ImplementationPlan{}, fmt.Errorf("milestone %d has no id", index)
		}
		if _, exists := seen[milestone.ID]; exists {
			return ImplementationPlan{}, fmt.Errorf("duplicate milestone id %q", milestone.ID)
		}
		seen[milestone.ID] = index
		if len(milestone.VerificationCommands) == 0 {
			return ImplementationPlan{}, fmt.Errorf("milestone %q has no verification commands", milestone.ID)
		}
		for _, command := range milestone.VerificationCommands {
			if err := ValidateCommandSpec(command); err != nil {
				return ImplementationPlan{}, fmt.Errorf("milestone %q: %w", milestone.ID, err)
			}
		}
	}
	for milestoneIndex, milestone := range plan.Milestones {
		for _, dependency := range milestone.Dependencies {
			dependencyIndex, exists := seen[dependency]
			if !exists {
				return ImplementationPlan{}, fmt.Errorf(
					"milestone %q references unknown dependency %q",
					milestone.ID,
					dependency,
				)
			}
			if dependencyIndex >= milestoneIndex {
				return ImplementationPlan{}, fmt.Errorf(
					"milestone %q dependency %q must appear earlier in the plan",
					milestone.ID,
					dependency,
				)
			}
		}
	}
	return plan, nil
}

func ValidateCommandSpec(command CommandSpec) error {
	executable := strings.TrimSpace(command.Executable)
	if executable == "" {
		return fmt.Errorf("verification executable is required")
	}
	if strings.ContainsAny(executable, "/\\") {
		return fmt.Errorf("verification executable %q must be resolved from PATH", executable)
	}
	if strings.ContainsAny(executable, " \t\r\n") {
		return fmt.Errorf("verification executable %q must not contain whitespace", executable)
	}
	switch executable {
	case "sh", "bash", "zsh", "fish", "dash", "sudo", "doas", "env":
		return fmt.Errorf("verification executable %q is not allowed", executable)
	}
	for _, arg := range command.Args {
		if strings.ContainsRune(arg, '\x00') {
			return fmt.Errorf("verification argument contains NUL")
		}
	}
	return nil
}
