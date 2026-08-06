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
	streamQueueBytes         = 4 * 1024 * 1024
	streamSaturationLimit    = 5 * time.Second
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

	processContext, cancelProcess := context.WithCancel(ctx)
	defer cancelProcess()
	command := exec.CommandContext(processContext, request.Executable, request.Args...)
	command.Dir = request.Directory
	command.Stdin = bytes.NewReader(request.Stdin)
	if request.Env == nil {
		command.Env = os.Environ()
	} else {
		command.Env = request.Env
	}

	stdout := newTailBuffer(diagnosticTailBytes)
	stderr := newTailBuffer(diagnosticTailBytes)
	stdoutSink := newStreamSink("stdout", io.MultiWriter(stdout, writerOrDiscard(request.Stdout)), request.OnOutput, cancelProcess)
	stderrSink := newStreamSink("stderr", io.MultiWriter(stderr, writerOrDiscard(request.Stderr)), request.OnOutput, cancelProcess)
	command.Stdout = stdoutSink
	command.Stderr = stderrSink

	if err := command.Start(); err != nil {
		_ = stdoutSink.Close()
		_ = stderrSink.Close()
		return ProcessResult{ExitCode: -1}, fmt.Errorf("start process: %w", err)
	}
	interval := request.HeartbeatInterval
	if interval <= 0 {
		interval = defaultHeartbeatInterval
	}
	done := make(chan struct{})
	heartbeatDone := make(chan struct{})
	if request.OnHeartbeat != nil {
		go func() {
			defer close(heartbeatDone)
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
	} else {
		close(heartbeatDone)
	}
	err := command.Wait()
	close(done)
	<-heartbeatDone
	stdoutErr := stdoutSink.Close()
	stderrErr := stderrSink.Close()
	if stdoutErr != nil {
		err = stdoutErr
	}
	if stderrErr != nil {
		err = stderrErr
	}
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

type streamSink struct {
	stream      string
	writer      io.Writer
	observe     func(string, time.Time)
	cancel      context.CancelFunc
	queue       chan []byte
	done        chan struct{}
	mu          sync.Mutex
	queuedBytes int
	err         error
	closeOnce   sync.Once
}

func newStreamSink(stream string, writer io.Writer, observe func(string, time.Time), cancel context.CancelFunc) *streamSink {
	sink := &streamSink{
		stream: stream, writer: writer, observe: observe, cancel: cancel,
		queue: make(chan []byte, 256), done: make(chan struct{}),
	}
	go sink.consume()
	return sink
}

func (s *streamSink) Write(data []byte) (int, error) {
	chunk := append([]byte(nil), data...)
	deadline := time.NewTimer(streamSaturationLimit)
	defer deadline.Stop()
	for {
		s.mu.Lock()
		if s.err != nil {
			err := s.err
			s.mu.Unlock()
			return 0, err
		}
		if s.queuedBytes+len(chunk) <= streamQueueBytes {
			s.queuedBytes += len(chunk)
			s.mu.Unlock()
			break
		}
		s.mu.Unlock()
		select {
		case <-deadline.C:
			err := errors.New("LOG_SINK_BACKPRESSURE: streamed log sink remained saturated")
			s.fail(err)
			return 0, err
		case <-time.After(time.Millisecond):
		}
	}
	select {
	case s.queue <- chunk:
		return len(data), nil
	case <-deadline.C:
		s.mu.Lock()
		s.queuedBytes -= len(chunk)
		s.mu.Unlock()
		err := errors.New("LOG_SINK_BACKPRESSURE: streamed log queue remained saturated")
		s.fail(err)
		return 0, err
	}
}

func (s *streamSink) consume() {
	defer close(s.done)
	for chunk := range s.queue {
		n, err := s.writer.Write(chunk)
		s.mu.Lock()
		s.queuedBytes -= len(chunk)
		s.mu.Unlock()
		if err != nil || n != len(chunk) {
			if err == nil {
				err = io.ErrShortWrite
			}
			s.fail(fmt.Errorf("LOG_SINK_WRITE_FAILED: %w", err))
			return
		}
		if s.observe != nil {
			s.observe(s.stream, time.Now().UTC())
		}
	}
}

func (s *streamSink) fail(err error) {
	s.mu.Lock()
	if s.err == nil {
		s.err = err
		s.cancel()
	}
	s.mu.Unlock()
}

func (s *streamSink) Close() error {
	s.closeOnce.Do(func() {
		close(s.queue)
		<-s.done
	})
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
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
