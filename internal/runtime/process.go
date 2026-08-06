package runtime

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

const (
	defaultHeartbeatInterval = 10 * time.Second
	diagnosticTailBytes      = 2 * 1024 * 1024
)

type ProcessRequest struct {
	Executable        string
	Args              []string
	Directory         string
	Stdin             []byte
	Env               []string
	Stdout            io.Writer
	Stderr            io.Writer
	HeartbeatInterval time.Duration
	OnHeartbeat       func(time.Time)
	OnOutput          func(string, time.Time)
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

	stdout := newTailBuffer(diagnosticTailBytes)
	stderr := newTailBuffer(diagnosticTailBytes)
	command.Stdout = observedWriter("stdout", io.MultiWriter(stdout, writerOrDiscard(request.Stdout)), request.OnOutput)
	command.Stderr = observedWriter("stderr", io.MultiWriter(stderr, writerOrDiscard(request.Stderr)), request.OnOutput)

	if err := command.Start(); err != nil {
		return ProcessResult{ExitCode: -1}, fmt.Errorf("start process: %w", err)
	}
	interval := request.HeartbeatInterval
	if interval <= 0 {
		interval = defaultHeartbeatInterval
	}
	done := make(chan struct{})
	if request.OnHeartbeat != nil {
		go func() {
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				select {
				case at := <-ticker.C:
					request.OnHeartbeat(at.UTC())
				case <-done:
					return
				}
			}
		}()
	}
	err := command.Wait()
	close(done)
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
	return result, fmt.Errorf("wait for process: %w", err)
}

func writerOrDiscard(writer io.Writer) io.Writer {
	if writer == nil {
		return io.Discard
	}
	return writer
}

type outputObserver struct {
	stream  string
	writer  io.Writer
	observe func(string, time.Time)
}

func observedWriter(stream string, writer io.Writer, observe func(string, time.Time)) io.Writer {
	return &outputObserver{stream: stream, writer: writer, observe: observe}
}

func (w *outputObserver) Write(data []byte) (int, error) {
	n, err := w.writer.Write(data)
	if n > 0 && w.observe != nil {
		w.observe(w.stream, time.Now().UTC())
	}
	return n, err
}

type tailBuffer struct {
	mu    sync.Mutex
	limit int
	data  []byte
}

func newTailBuffer(limit int) *tailBuffer { return &tailBuffer{limit: limit} }

func (b *tailBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(data) >= b.limit {
		b.data = append(b.data[:0], data[len(data)-b.limit:]...)
	} else {
		overflow := len(b.data) + len(data) - b.limit
		if overflow > 0 {
			copy(b.data, b.data[overflow:])
			b.data = b.data[:len(b.data)-overflow]
		}
		b.data = append(b.data, data...)
	}
	return len(data), nil
}

func (b *tailBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]byte(nil), b.data...)
}
