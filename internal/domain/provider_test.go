package domain

import (
	"errors"
	"testing"
)

func TestResolveRoles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     RoleInputs
		want      ResolvedRoles
		wantError error
	}{
		{
			name:  "all defaults",
			input: RoleInputs{},
			want: ResolvedRoles{
				Designer:    ProviderCodex,
				Implementer: ProviderCodex,
				Reviewer:    ProviderClaude,
			},
		},
		{
			name: "designer claude derives implementer and reviewer",
			input: RoleInputs{
				Designer: "claude",
			},
			want: ResolvedRoles{
				Designer:    ProviderClaude,
				Implementer: ProviderClaude,
				Reviewer:    ProviderCodex,
			},
		},
		{
			name: "explicit valid assignment",
			input: RoleInputs{
				Designer:    "claude",
				Implementer: "claude",
				Reviewer:    "codex",
			},
			want: ResolvedRoles{
				Designer:    ProviderClaude,
				Implementer: ProviderClaude,
				Reviewer:    ProviderCodex,
			},
		},
		{
			name: "reviewer cannot equal designer",
			input: RoleInputs{
				Designer: "claude",
				Reviewer: "claude",
			},
			wantError: ErrRoleAssignmentConflict,
		},
		{
			name: "two different producers leave no independent reviewer",
			input: RoleInputs{
				Designer:    "codex",
				Implementer: "claude",
			},
			wantError: ErrRoleAssignmentConflict,
		},
		{
			name: "unsupported provider",
			input: RoleInputs{
				Designer: "gemini",
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ResolveRoles(tt.input)
			if tt.wantError != nil {
				if !errors.Is(err, tt.wantError) {
					t.Fatalf("ResolveRoles() error = %v, want %v", err, tt.wantError)
				}
				return
			}
			if tt.name == "unsupported provider" {
				if err == nil {
					t.Fatal("ResolveRoles() error = nil, want an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveRoles() unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("ResolveRoles() = %#v, want %#v", got, tt.want)
			}
		})
	}
}
