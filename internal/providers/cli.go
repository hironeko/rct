package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hironeko/loop-engine/internal/domain"
	loopruntime "github.com/hironeko/loop-engine/internal/runtime"
)

type CLIGateway struct {
	runner loopruntime.ProcessRunner
}

func NewCLIGateway(runner loopruntime.ProcessRunner) *CLIGateway {
	return &CLIGateway{runner: runner}
}

func (g *CLIGateway) Execute(ctx context.Context, job Job) (Result, error) {
	if g.runner == nil {
		return Result{}, errors.New("process runner is required")
	}
	if err := validateJob(job); err != nil {
		return Result{}, err
	}
	outputSchema, err := compileOutputSchema(job.Schema)
	if err != nil {
		return Result{}, fmt.Errorf("compile job output schema: %w", err)
	}
	if err := os.MkdirAll(job.JobDir, 0o700); err != nil {
		return Result{}, fmt.Errorf("create job directory: %w", err)
	}

	schemaPath := filepath.Join(job.JobDir, "output.schema.json")
	promptPath := filepath.Join(job.JobDir, "prompt.md")
	if err := os.WriteFile(schemaPath, job.Schema, 0o600); err != nil {
		return Result{}, fmt.Errorf("write job schema: %w", err)
	}
	if err := os.WriteFile(promptPath, job.Prompt, 0o600); err != nil {
		return Result{}, fmt.Errorf("write job prompt: %w", err)
	}

	var (
		result  loopruntime.ProcessResult
		output  []byte
		execErr error
	)
	switch job.Provider {
	case domain.ProviderCodex:
		result, output, execErr = g.executeCodex(ctx, job, schemaPath)
	case domain.ProviderClaude:
		result, output, execErr = g.executeClaude(ctx, job)
	default:
		return Result{}, fmt.Errorf("unsupported provider %q", job.Provider)
	}

	_ = os.WriteFile(filepath.Join(job.JobDir, "stdout.log"), result.Stdout, 0o600)
	_ = os.WriteFile(filepath.Join(job.JobDir, "stderr.log"), result.Stderr, 0o600)
	if execErr != nil {
		return Result{Stdout: result.Stdout, Stderr: result.Stderr}, fmt.Errorf(
			"%s job %s: %w; inspect %s",
			job.Provider,
			job.ID,
			execErr,
			job.JobDir,
		)
	}
	if !json.Valid(output) {
		return Result{Stdout: result.Stdout, Stderr: result.Stderr}, fmt.Errorf(
			"%s job %s returned invalid JSON; inspect %s",
			job.Provider,
			job.ID,
			job.JobDir,
		)
	}
	if err := validateStructuredOutput(outputSchema, output); err != nil {
		_ = os.WriteFile(
			filepath.Join(job.JobDir, "invalid-structured-output.json"),
			output,
			0o600,
		)
		return Result{Stdout: result.Stdout, Stderr: result.Stderr}, fmt.Errorf(
			"%s job %s returned JSON that does not satisfy its schema: %w; inspect %s",
			job.Provider,
			job.ID,
			err,
			job.JobDir,
		)
	}
	if err := os.WriteFile(filepath.Join(job.JobDir, "structured-output.json"), output, 0o600); err != nil {
		return Result{}, fmt.Errorf("write structured output: %w", err)
	}
	return Result{
		StructuredOutput: output,
		Stdout:           result.Stdout,
		Stderr:           result.Stderr,
	}, nil
}

func (g *CLIGateway) executeCodex(
	ctx context.Context,
	job Job,
	schemaPath string,
) (loopruntime.ProcessResult, []byte, error) {
	outputPath := filepath.Join(job.JobDir, "codex-final.json")
	sandbox := "read-only"
	if job.Access == AccessWorkspaceWrite {
		sandbox = "workspace-write"
	}
	result, err := g.runner.Run(ctx, loopruntime.ProcessRequest{
		Executable: domain.ProviderCodex.Executable(),
		Args: []string{
			"exec",
			"--ephemeral",
			"--ignore-user-config",
			"--ignore-rules",
			"--sandbox", sandbox,
			"--skip-git-repo-check",
			"--output-schema", schemaPath,
			"--output-last-message", outputPath,
			"--cd", job.Project,
			"-",
		},
		Directory: job.Project,
		Stdin:     job.Prompt,
	})
	if err != nil {
		return result, nil, err
	}
	output, readErr := os.ReadFile(outputPath)
	if readErr != nil {
		return result, nil, fmt.Errorf("read Codex final output: %w", readErr)
	}
	return result, output, nil
}

func (g *CLIGateway) executeClaude(
	ctx context.Context,
	job Job,
) (loopruntime.ProcessResult, []byte, error) {
	permissionMode := "dontAsk"
	tools := "Read,Glob,Grep"
	if job.Access == AccessWorkspaceWrite {
		permissionMode = "acceptEdits"
		tools = "Read,Glob,Grep,Edit,Write"
	}
	result, err := g.runner.Run(ctx, loopruntime.ProcessRequest{
		Executable: domain.ProviderClaude.Executable(),
		Args: []string{
			"--print",
			"--safe-mode",
			"--output-format", "json",
			"--json-schema", string(job.Schema),
			"--permission-mode", permissionMode,
			"--tools=" + tools,
			"--no-chrome",
			"--no-session-persistence",
		},
		Directory: job.Project,
		Stdin:     job.Prompt,
	})
	if err != nil {
		return result, nil, err
	}

	var envelope struct {
		StructuredOutput json.RawMessage `json:"structured_output"`
		Result           string          `json:"result"`
	}
	if err := json.Unmarshal(result.Stdout, &envelope); err != nil {
		return result, nil, fmt.Errorf("decode Claude result envelope: %w", err)
	}
	if len(envelope.StructuredOutput) > 0 && string(envelope.StructuredOutput) != "null" {
		return result, envelope.StructuredOutput, nil
	}
	if json.Valid([]byte(envelope.Result)) {
		return result, []byte(envelope.Result), nil
	}
	return result, nil, errors.New("Claude result did not contain structured_output")
}

func validateJob(job Job) error {
	if strings.TrimSpace(job.ID) == "" {
		return errors.New("job id is required")
	}
	if job.Provider != domain.ProviderCodex && job.Provider != domain.ProviderClaude {
		return fmt.Errorf("unsupported provider %q", job.Provider)
	}
	if job.Role != domain.RoleDesigner && job.Role != domain.RoleReviewer &&
		job.Role != domain.RoleImplementer {
		return fmt.Errorf("unsupported role %q for structured workflow", job.Role)
	}
	if job.Access == "" {
		job.Access = AccessReadOnly
	}
	if job.Access != AccessReadOnly && job.Access != AccessWorkspaceWrite {
		return fmt.Errorf("unsupported job access mode %q", job.Access)
	}
	if job.Access == AccessWorkspaceWrite && job.Role != domain.RoleImplementer {
		return fmt.Errorf("workspace-write access is only valid for implementer jobs")
	}
	if strings.TrimSpace(job.Project) == "" {
		return errors.New("project directory is required")
	}
	if strings.TrimSpace(job.JobDir) == "" {
		return errors.New("job directory is required")
	}
	if len(job.Prompt) == 0 {
		return errors.New("job prompt is required")
	}
	if !json.Valid(job.Schema) {
		return errors.New("job schema must be valid JSON")
	}
	return nil
}
