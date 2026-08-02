package agentassets

import (
	"embed"
	"fmt"
)

// content contains provider-neutral role contracts and reusable skills.
//
//go:embed roles/*.md shared/*.md skills/*/SKILL.md
var content embed.FS

func DesignerInstructions() (string, error) {
	return join(
		"shared/safety-rules.md",
		"roles/designer.md",
		"skills/design-requirements/SKILL.md",
	)
}

func ReviewerInstructions() (string, error) {
	return join(
		"shared/safety-rules.md",
		"roles/reviewer.md",
		"skills/review-artifact/SKILL.md",
	)
}

func ImplementerInstructions() (string, error) {
	return join(
		"shared/safety-rules.md",
		"roles/implementer.md",
	)
}

func join(paths ...string) (string, error) {
	result := ""
	for _, path := range paths {
		data, err := content.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read embedded agent asset %q: %w", path, err)
		}
		if result != "" {
			result += "\n\n"
		}
		result += string(data)
	}
	return result, nil
}
