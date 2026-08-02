package domain

import (
	"errors"
	"fmt"
	"strings"
)

type Provider string

const (
	ProviderCodex  Provider = "codex"
	ProviderClaude Provider = "claude"
)

var ErrRoleAssignmentConflict = errors.New("role assignment conflict")

func ParseProvider(value string) (Provider, error) {
	switch Provider(strings.ToLower(strings.TrimSpace(value))) {
	case ProviderCodex:
		return ProviderCodex, nil
	case ProviderClaude:
		return ProviderClaude, nil
	default:
		return "", fmt.Errorf("unsupported provider %q: expected codex or claude", value)
	}
}

func (p Provider) Executable() string {
	switch p {
	case ProviderCodex:
		return "codex"
	case ProviderClaude:
		return "claude"
	default:
		return ""
	}
}

func (p Provider) Other() (Provider, error) {
	switch p {
	case ProviderCodex:
		return ProviderClaude, nil
	case ProviderClaude:
		return ProviderCodex, nil
	default:
		return "", fmt.Errorf("provider %q has no configured counterpart", p)
	}
}

type Role string

const (
	RoleDesigner    Role = "designer"
	RoleImplementer Role = "implementer"
	RoleReviewer    Role = "reviewer"
)

type RoleInputs struct {
	Designer    string
	Implementer string
	Reviewer    string
}

type ResolvedRoles struct {
	Designer    Provider `json:"designer"`
	Implementer Provider `json:"implementer"`
	Reviewer    Provider `json:"reviewer"`
}

// ResolveRoles implements FR-003 and FR-154. Defaults are derived in order:
// designer -> implementer -> reviewer.
func ResolveRoles(input RoleInputs) (ResolvedRoles, error) {
	designerValue := input.Designer
	if strings.TrimSpace(designerValue) == "" {
		designerValue = string(ProviderCodex)
	}
	designer, err := ParseProvider(designerValue)
	if err != nil {
		return ResolvedRoles{}, fmt.Errorf("designer: %w", err)
	}

	implementerValue := input.Implementer
	if strings.TrimSpace(implementerValue) == "" {
		implementerValue = string(designer)
	}
	implementer, err := ParseProvider(implementerValue)
	if err != nil {
		return ResolvedRoles{}, fmt.Errorf("implementer: %w", err)
	}

	reviewerValue := input.Reviewer
	if strings.TrimSpace(reviewerValue) == "" {
		other, otherErr := designer.Other()
		if otherErr != nil {
			return ResolvedRoles{}, otherErr
		}
		reviewerValue = string(other)
	}
	reviewer, err := ParseProvider(reviewerValue)
	if err != nil {
		return ResolvedRoles{}, fmt.Errorf("reviewer: %w", err)
	}

	resolved := ResolvedRoles{
		Designer:    designer,
		Implementer: implementer,
		Reviewer:    reviewer,
	}
	if err := resolved.Validate(); err != nil {
		return ResolvedRoles{}, err
	}
	return resolved, nil
}

func (r ResolvedRoles) Validate() error {
	if r.Reviewer == r.Designer {
		return fmt.Errorf(
			"%w: reviewer provider %q equals designer provider; choose an independent reviewer",
			ErrRoleAssignmentConflict,
			r.Reviewer,
		)
	}
	if r.Reviewer == r.Implementer {
		return fmt.Errorf(
			"%w: reviewer provider %q equals implementer provider; with two providers designer and implementer must match",
			ErrRoleAssignmentConflict,
			r.Reviewer,
		)
	}
	return nil
}

func (r ResolvedRoles) Providers() []Provider {
	seen := make(map[Provider]bool, 2)
	result := make([]Provider, 0, 2)
	for _, provider := range []Provider{r.Designer, r.Implementer, r.Reviewer} {
		if !seen[provider] {
			seen[provider] = true
			result = append(result, provider)
		}
	}
	return result
}
