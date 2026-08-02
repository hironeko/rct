package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hironeko/rct/internal/app"
	"github.com/hironeko/rct/internal/domain"
	"github.com/hironeko/rct/internal/providers"
)

type cliGateway struct {
	outputs [][]byte
}

func (g *cliGateway) Execute(
	_ context.Context,
	_ providers.Job,
) (providers.Result, error) {
	output := g.outputs[0]
	g.outputs = g.outputs[1:]
	return providers.Result{StructuredOutput: output}, nil
}

func TestStartDesignerClaude(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	service := app.NewService(app.Dependencies{
		LookPath: func(name string) (string, error) {
			switch name {
			case "codex", "claude":
				return "/usr/local/bin/" + name, nil
			default:
				return "", errors.New("not found")
			}
		},
		Getenv: func(string) string { return "" },
		Now: func() time.Time {
			return time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
		},
		Random: bytes.NewReader([]byte{1, 2, 3, 4, 5, 6}),
	})
	command := New(service, strings.NewReader(""), &stdout, &stderr)

	exitCode := command.Run(context.Background(), []string{
		"start",
		"--project", t.TempDir(),
		"--backend", "direct",
		"--designer", "claude",
		"--request", "Build rct",
	})
	if exitCode != 0 {
		t.Fatalf("Run() exit code = %d, stderr = %q", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "designer=claude implementer=claude reviewer=codex") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestStartRejectsConflictingRequestSources(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command := New(
		app.NewService(app.DefaultDependencies()),
		strings.NewReader(""),
		&stdout,
		&stderr,
	)
	exitCode := command.Run(context.Background(), []string{
		"start",
		"--request", "one",
		"two",
	})
	if exitCode != 2 {
		t.Fatalf("Run() exit code = %d, want 2", exitCode)
	}
}

func TestStartExecuteRunsDesignWorkflow(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	now := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	runID := "run_20260729T120000Z_010203040506"
	requirements := []byte(`{
		"schema_version":"1.0",
		"title":"rct",
		"summary":"Summary",
		"problem_statement":"Problem",
		"goals":["Goal"],
		"non_goals":[],
		"assumptions":[],
		"constraints":[],
		"requirements":[{
			"id":"REQ-001",
			"title":"Run",
			"statement":"Run the loop",
			"priority":"must",
			"acceptance_criteria":["The loop completes"]
		}],
		"risks":[],
		"open_questions":[]
	}`)
	hashData := append(append([]byte{}, requirements...), '\n')
	subjectHash := sha256.Sum256(hashData)
	subjectPath := filepath.ToSlash(filepath.Join(
		".rct",
		"runs",
		runID,
		"artifacts",
		"requirements",
		"v001.json",
	))
	gateway := &cliGateway{
		outputs: [][]byte{
			requirements,
			[]byte(fmt.Sprintf(`{
				"schema_version":"1.0",
				"run_id":%q,
				"job_id":"requirements-r01-reviewer",
				"review_type":"requirements",
				"subject":{"path":%q,"sha256":%q,"media_type":"application/json"},
				"verdict":"approved",
				"summary":"Approved",
				"scores":{
					"clarity":5,
					"completeness":5,
					"feasibility":5,
					"testability":5,
					"risk_control":5
				},
				"required_changes":[],
				"optional_suggestions":[],
				"open_questions":[]
			}`, runID, subjectPath, hex.EncodeToString(subjectHash[:]))),
		},
	}
	service := app.NewService(app.Dependencies{
		LookPath: func(name string) (string, error) {
			return "/usr/local/bin/" + name, nil
		},
		Getenv:       func(string) string { return "" },
		Now:          func() time.Time { return now },
		Random:       bytes.NewReader([]byte{1, 2, 3, 4, 5, 6}),
		Agent:        gateway,
		ProviderAuth: func(context.Context, domain.Provider) error { return nil },
	})
	command := New(service, strings.NewReader(""), &stdout, &stderr)
	exitCode := command.Run(context.Background(), []string{
		"start",
		"--project", t.TempDir(),
		"--backend", "direct",
		"--mode", "design-only",
		"--request", "Build rct",
		"--execute",
		"--until", "requirements",
	})
	if exitCode != 0 {
		t.Fatalf("Run() exit code = %d, stderr = %q", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "State: REQUIREMENTS_APPROVED") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}
