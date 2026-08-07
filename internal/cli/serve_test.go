package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hironeko/rct/internal/app"
)

func TestServeStartsOnLoopbackAndStopsWithContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var stdout, stderr bytes.Buffer
	command := New(app.NewService(app.DefaultDependencies()), strings.NewReader(""), &stdout, &stderr)
	code := command.Run(ctx, []string{
		"serve", "--workspace-root", t.TempDir(), "--listen", "127.0.0.1:0", "--no-open", "--json",
	})
	if code != 0 {
		t.Fatalf("serve exit code = %d, stderr = %q", code, stderr.String())
	}
	var output struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		t.Fatalf("serve JSON = %q: %v", stdout.String(), err)
	}
	if !strings.HasPrefix(output.URL, "http://127.0.0.1:") || strings.Contains(output.URL, "bootstrap") {
		t.Fatalf("safe URL = %q", output.URL)
	}
	if !strings.Contains(stderr.String(), "one-time local bootstrap URL") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestServeRejectsRemoteListen(t *testing.T) {
	var stdout, stderr bytes.Buffer
	command := New(app.NewService(app.DefaultDependencies()), strings.NewReader(""), &stdout, &stderr)
	code := command.Run(context.Background(), []string{
		"serve", "--workspace-root", t.TempDir(), "--listen", "0.0.0.0:8080", "--no-open",
	})
	if code != 1 || !strings.Contains(stderr.String(), "must use 127.0.0.1") {
		t.Fatalf("serve exit code = %d, stderr = %q", code, stderr.String())
	}
}
