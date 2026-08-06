package runtime

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"
)

type signalWriter struct {
	mu    sync.Mutex
	buf   bytes.Buffer
	first chan struct{}
	once  sync.Once
}

func (w *signalWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n, err := w.buf.Write(data)
	w.once.Do(func() { close(w.first) })
	return n, err
}

func TestDirectProcessRunnerStreamsBeforeExitAndHeartbeats(t *testing.T) {
	writer := &signalWriter{first: make(chan struct{})}
	heartbeats := make(chan time.Time, 4)
	done := make(chan error, 1)
	go func() {
		_, err := (DirectProcessRunner{}).Run(context.Background(), ProcessRequest{
			Executable: "sh", Args: []string{"-c", "printf first; sleep 0.12; printf second"},
			Stdout: writer, HeartbeatInterval: 20 * time.Millisecond,
			OnHeartbeat: func(at time.Time) {
				select {
				case heartbeats <- at:
				default:
				}
			},
		})
		done <- err
	}()

	select {
	case <-writer.first:
	case <-time.After(time.Second):
		t.Fatal("stdout was not streamed before timeout")
	}
	select {
	case <-heartbeats:
	case <-time.After(time.Second):
		t.Fatal("heartbeat was not emitted while process was alive")
	}
	if err := <-done; err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	writer.mu.Lock()
	got := writer.buf.String()
	writer.mu.Unlock()
	if got != "firstsecond" {
		t.Fatalf("streamed stdout = %q", got)
	}
}

func TestTailBufferIsBounded(t *testing.T) {
	buffer := newTailBuffer(4)
	_, _ = buffer.Write([]byte("abcdef"))
	_, _ = buffer.Write([]byte("gh"))
	if got := string(buffer.Bytes()); got != "efgh" {
		t.Fatalf("tail = %q, want efgh", got)
	}
}
