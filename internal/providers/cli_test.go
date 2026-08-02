package providers

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/hironeko/loop-engine/internal/domain"
	loopruntime "github.com/hironeko/loop-engine/internal/runtime"
)

type fakeRunner struct {
	request loopruntime.ProcessRequest
	run     func(loopruntime.ProcessRequest) (loopruntime.ProcessResult, error)
}

func (f *fakeRunner) Run(
	_ context.Context,
	request loopruntime.ProcessRequest,
) (loopruntime.ProcessResult, error) {
	f.request = request
	return f.run(request)
}

func TestCLIGatewayExecutesCodexWithReadOnlyStructuredOutput(t *testing.T) {
	t.Parallel()

	jobDir := t.TempDir()
	expected := []byte(`{"schema_version":"1.0"}`)
	runner := &fakeRunner{
		run: func(request loopruntime.ProcessRequest) (loopruntime.ProcessResult, error) {
			outputIndex := slices.Index(request.Args, "--output-last-message")
			if outputIndex < 0 || outputIndex+1 >= len(request.Args) {
				t.Fatal("Codex args do not contain output path")
			}
			if err := os.WriteFile(request.Args[outputIndex+1], expected, 0o600); err != nil {
				t.Fatalf("write fake Codex output: %v", err)
			}
			return loopruntime.ProcessResult{Stdout: expected}, nil
		},
	}

	result, err := NewCLIGateway(runner).Execute(context.Background(), Job{
		ID:       "requirements-r01-designer",
		Provider: domain.ProviderCodex,
		Role:     domain.RoleDesigner,
		Project:  t.TempDir(),
		JobDir:   jobDir,
		Prompt:   []byte("design"),
		Schema:   []byte(`{"type":"object"}`),
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if string(result.StructuredOutput) != string(expected) {
		t.Fatalf("output = %s, want %s", result.StructuredOutput, expected)
	}
	if !slices.Contains(runner.request.Args, "read-only") {
		t.Fatalf("Codex args = %#v, want read-only sandbox", runner.request.Args)
	}
}

func TestCLIGatewayExtractsClaudeStructuredOutput(t *testing.T) {
	t.Parallel()

	structured := json.RawMessage(`{"schema_version":"1.0","verdict":"approved"}`)
	envelope, err := json.Marshal(map[string]any{
		"structured_output": structured,
		"session_id":        "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{
		run: func(loopruntime.ProcessRequest) (loopruntime.ProcessResult, error) {
			return loopruntime.ProcessResult{Stdout: envelope}, nil
		},
	}

	result, err := NewCLIGateway(runner).Execute(context.Background(), Job{
		ID:       "requirements-r01-reviewer",
		Provider: domain.ProviderClaude,
		Role:     domain.RoleReviewer,
		Project:  t.TempDir(),
		JobDir:   t.TempDir(),
		Prompt:   []byte("review"),
		Schema:   []byte(`{"type":"object"}`),
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if string(result.StructuredOutput) != string(structured) {
		t.Fatalf("output = %s, want %s", result.StructuredOutput, structured)
	}
	if runner.request.Executable != "claude" {
		t.Fatalf("executable = %q, want claude", runner.request.Executable)
	}
	if !slices.Contains(runner.request.Args, "--safe-mode") {
		t.Fatalf("Claude args = %#v, want --safe-mode", runner.request.Args)
	}
	if runner.request.Directory == "" || filepath.Clean(runner.request.Directory) == "." {
		t.Fatalf("directory = %q, want explicit project", runner.request.Directory)
	}
}

func TestCLIGatewayRejectsOutputThatDoesNotSatisfySchema(t *testing.T) {
	t.Parallel()

	structured := json.RawMessage(`{"schema_version":"2.0","score":6}`)
	envelope, err := json.Marshal(map[string]any{
		"structured_output": structured,
	})
	if err != nil {
		t.Fatal(err)
	}
	jobDir := t.TempDir()
	runner := &fakeRunner{
		run: func(loopruntime.ProcessRequest) (loopruntime.ProcessResult, error) {
			return loopruntime.ProcessResult{Stdout: envelope}, nil
		},
	}

	_, err = NewCLIGateway(runner).Execute(context.Background(), Job{
		ID:       "requirements-r01-reviewer",
		Provider: domain.ProviderClaude,
		Role:     domain.RoleReviewer,
		Project:  t.TempDir(),
		JobDir:   jobDir,
		Prompt:   []byte("review"),
		Schema: []byte(`{
			"type":"object",
			"additionalProperties":false,
			"properties":{
				"schema_version":{"const":"1.0"},
				"score":{"type":"integer","minimum":1,"maximum":5}
			},
			"required":["schema_version","score"]
		}`),
	})
	if err == nil || !strings.Contains(err.Error(), "does not satisfy its schema") {
		t.Fatalf("Execute() error = %v, want schema validation failure", err)
	}
	if _, statErr := os.Stat(filepath.Join(jobDir, "invalid-structured-output.json")); statErr != nil {
		t.Fatalf("invalid output was not preserved for diagnosis: %v", statErr)
	}
}

func TestCLIGatewayRejectsInvalidOutputSchemaBeforeExecution(t *testing.T) {
	t.Parallel()

	called := false
	runner := &fakeRunner{
		run: func(loopruntime.ProcessRequest) (loopruntime.ProcessResult, error) {
			called = true
			return loopruntime.ProcessResult{}, nil
		},
	}
	_, err := NewCLIGateway(runner).Execute(context.Background(), Job{
		ID:       "requirements-r01-designer",
		Provider: domain.ProviderCodex,
		Role:     domain.RoleDesigner,
		Project:  t.TempDir(),
		JobDir:   t.TempDir(),
		Prompt:   []byte("design"),
		Schema:   []byte(`{"type":"not-a-json-schema-type"}`),
	})
	if err == nil || !strings.Contains(err.Error(), "compile job output schema") {
		t.Fatalf("Execute() error = %v, want invalid schema failure", err)
	}
	if called {
		t.Fatal("provider was executed with an invalid output schema")
	}
}

func TestCLIGatewayDoesNotFallbackToCodexStdout(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{
		run: func(loopruntime.ProcessRequest) (loopruntime.ProcessResult, error) {
			return loopruntime.ProcessResult{
				Stdout: []byte(`{"schema_version":"1.0"}`),
			}, nil
		},
	}

	_, err := NewCLIGateway(runner).Execute(context.Background(), Job{
		ID:       "requirements-r01-designer",
		Provider: domain.ProviderCodex,
		Role:     domain.RoleDesigner,
		Project:  t.TempDir(),
		JobDir:   t.TempDir(),
		Prompt:   []byte("design"),
		Schema:   []byte(`{"type":"object"}`),
	})
	if err == nil || !strings.Contains(err.Error(), "read Codex final output") {
		t.Fatalf("Execute() error = %v, want missing final output failure", err)
	}
}
