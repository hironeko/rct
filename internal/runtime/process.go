package runtime

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
)

type ProcessRequest struct {
	Executable string
	Args       []string
	Directory  string
	Stdin      []byte
	Env        []string
}

type ProcessResult struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

type ProcessRunner interface {
	Run(context.Context, ProcessRequest) (ProcessResult, error)
}

type DirectProcessRunner struct{}

func (DirectProcessRunner) Run(ctx context.Context, request ProcessRequest) (ProcessResult, error) {
	if request.Executable == "" {
		return ProcessResult{}, errors.New("process executable is required")
	}

	command := exec.CommandContext(ctx, request.Executable, request.Args...)
	command.Dir = request.Directory
	command.Stdin = bytes.NewReader(request.Stdin)
	if request.Env == nil {
		command.Env = os.Environ()
	} else {
		command.Env = request.Env
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr

	err := command.Run()
	result := ProcessResult{
		Stdout:   stdout.Bytes(),
		Stderr:   stderr.Bytes(),
		ExitCode: 0,
	}
	if err == nil {
		return result, nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return result, ctxErr
	}

	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		result.ExitCode = exitError.ExitCode()
		return result, fmt.Errorf("process exited with code %d", result.ExitCode)
	}
	result.ExitCode = -1
	return result, fmt.Errorf("start process: %w", err)
}
