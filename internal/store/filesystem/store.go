package filesystem

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hironeko/loop-engine/internal/domain"
)

const stateDirectory = ".loop-engine"

type Store struct {
	project string
}

func New(project string) *Store {
	return &Store{project: project}
}

func (s *Store) Create(run domain.Run, request string) error {
	base := filepath.Join(s.project, stateDirectory)
	runDir := filepath.Join(base, "runs", run.ID)

	if _, err := os.Stat(runDir); err == nil {
		return fmt.Errorf("run directory already exists: %s", runDir)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect run directory: %w", err)
	}
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		return fmt.Errorf("create run directory: %w", err)
	}

	state, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		return fmt.Errorf("encode run state: %w", err)
	}
	state = append(state, '\n')

	event := map[string]any{
		"seq":          1,
		"timestamp":    run.CreatedAt,
		"run_id":       run.ID,
		"type":         "RunStarted",
		"state_before": nil,
		"state_after":  run.State,
	}
	eventData, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("encode initial event: %w", err)
	}
	eventData = append(eventData, '\n')

	if err := writeAtomic(filepath.Join(runDir, "request.md"), []byte(request+"\n"), 0o600); err != nil {
		return fmt.Errorf("write request: %w", err)
	}
	if err := writeAtomic(filepath.Join(runDir, "state.json"), state, 0o600); err != nil {
		return fmt.Errorf("write state: %w", err)
	}
	if err := writeAtomic(filepath.Join(runDir, "events.jsonl"), eventData, 0o600); err != nil {
		return fmt.Errorf("write event log: %w", err)
	}
	if err := writeAtomic(filepath.Join(base, "current-run"), []byte(run.ID+"\n"), 0o600); err != nil {
		return fmt.Errorf("write current run reference: %w", err)
	}
	return nil
}

func (s *Store) LoadCurrent() (domain.Run, error) {
	base := filepath.Join(s.project, stateDirectory)
	current, err := os.ReadFile(filepath.Join(base, "current-run"))
	if err != nil {
		return domain.Run{}, fmt.Errorf("read current run reference: %w", err)
	}
	runID := string(current)
	for len(runID) > 0 && (runID[len(runID)-1] == '\n' || runID[len(runID)-1] == '\r') {
		runID = runID[:len(runID)-1]
	}
	if runID == "" {
		return domain.Run{}, errors.New("current run reference is empty")
	}

	state, err := os.ReadFile(filepath.Join(base, "runs", runID, "state.json"))
	if err != nil {
		return domain.Run{}, fmt.Errorf("read current run state: %w", err)
	}
	var run domain.Run
	if err := json.Unmarshal(state, &run); err != nil {
		return domain.Run{}, fmt.Errorf("decode current run state: %w", err)
	}
	return run, nil
}

func (s *Store) LoadRequest(runID string) ([]byte, error) {
	request, err := os.ReadFile(filepath.Join(s.runDir(runID), "request.md"))
	if err != nil {
		return nil, fmt.Errorf("read run request: %w", err)
	}
	return request, nil
}

func (s *Store) RunDir(runID string) string {
	return s.runDir(runID)
}

func (s *Store) WriteRunFile(runID, relativePath string, data []byte) (string, error) {
	clean := filepath.Clean(relativePath)
	if clean == "." || filepath.IsAbs(clean) || clean == ".." ||
		strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid run-relative path %q", relativePath)
	}
	path := filepath.Join(s.runDir(runID), clean)
	if err := writeAtomic(path, appendNewline(data), 0o600); err != nil {
		return "", fmt.Errorf("write run file %q: %w", relativePath, err)
	}
	return filepath.ToSlash(filepath.Join(".loop-engine", "runs", runID, clean)), nil
}

func (s *Store) ReadRunFile(runID, relativePath string) ([]byte, error) {
	clean := filepath.Clean(relativePath)
	if clean == "." || filepath.IsAbs(clean) || clean == ".." ||
		strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("invalid run-relative path %q", relativePath)
	}
	data, err := os.ReadFile(filepath.Join(s.runDir(runID), clean))
	if err != nil {
		return nil, fmt.Errorf("read run file %q: %w", relativePath, err)
	}
	return data, nil
}

func (s *Store) ReadArtifact(runID, logicalPath string) ([]byte, error) {
	prefix := filepath.ToSlash(filepath.Join(stateDirectory, "runs", runID)) + "/"
	logical := filepath.ToSlash(filepath.Clean(logicalPath))
	if !strings.HasPrefix(logical, prefix) {
		return nil, fmt.Errorf("artifact path %q is outside run %q", logicalPath, runID)
	}
	return s.ReadRunFile(runID, strings.TrimPrefix(logical, prefix))
}

func (s *Store) Update(run domain.Run, previous domain.WorkflowState, eventType string) error {
	state, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		return fmt.Errorf("encode run state: %w", err)
	}
	if err := writeAtomic(
		filepath.Join(s.runDir(run.ID), "state.json"),
		appendNewline(state),
		0o600,
	); err != nil {
		return fmt.Errorf("write run state: %w", err)
	}

	event := map[string]any{
		"timestamp":    run.UpdatedAt,
		"run_id":       run.ID,
		"type":         eventType,
		"state_before": previous,
		"state_after":  run.State,
		"revision":     run.Revision,
	}
	eventData, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("encode run event: %w", err)
	}
	logPath := filepath.Join(s.runDir(run.ID), "events.jsonl")
	file, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open event log: %w", err)
	}
	if _, err := file.Write(appendNewline(eventData)); err != nil {
		_ = file.Close()
		return fmt.Errorf("append event log: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync event log: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close event log: %w", err)
	}
	return nil
}

func (s *Store) runDir(runID string) string {
	return filepath.Join(s.project, stateDirectory, "runs", runID)
}

func appendNewline(data []byte) []byte {
	if len(data) == 0 || data[len(data)-1] != '\n' {
		result := make([]byte, len(data)+1)
		copy(result, data)
		result[len(result)-1] = '\n'
		return result
	}
	return data
}

func writeAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	file, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tempPath := file.Name()
	cleanup := func() {
		_ = file.Close()
		_ = os.Remove(tempPath)
	}

	if err := file.Chmod(mode); err != nil {
		cleanup()
		return err
	}
	if _, err := file.Write(data); err != nil {
		cleanup()
		return err
	}
	if err := file.Sync(); err != nil {
		cleanup()
		return err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	return nil
}
