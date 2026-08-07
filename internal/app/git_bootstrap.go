package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hironeko/rct/internal/store/filesystem"
)

const (
	RepositoryExisting        = "existing_repository"
	RepositoryManagedMinimal  = "managed_minimal_uninitialized"
	RepositoryUnmanaged       = "unmanaged_uninitialized"
	RepositoryUnborn          = "unborn_repository"
	RepositoryUnsafeBoundary  = "unsafe_repository_boundary"
	defaultBootstrapCommitMsg = "chore: initialize project for rct"
	maxBootstrapFileBytes     = 16 * 1024 * 1024
	maxBootstrapTotalBytes    = 64 * 1024 * 1024
)

type BaselineEntry struct {
	Path   string `json:"path"`
	Kind   string `json:"kind"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
	Mode   string `json:"mode"`
}

type GitBootstrapPlan struct {
	SchemaVersion   string          `json:"schema_version"`
	ID              string          `json:"id"`
	Project         string          `json:"project"`
	RepositoryClass string          `json:"repository_class"`
	AdoptExisting   bool            `json:"adopt_existing"`
	Inventory       []BaselineEntry `json:"inventory"`
	InventorySHA256 string          `json:"inventory_sha256"`
	GitignoreBefore string          `json:"gitignore_before_sha256,omitempty"`
	GitignoreAfter  string          `json:"gitignore_after_sha256"`
	CommitMessage   string          `json:"commit_message"`
	CreatedAt       time.Time       `json:"created_at"`
}

type GitBootstrapReceipt struct {
	SchemaVersion   string          `json:"schema_version"`
	PlanID          string          `json:"plan_id"`
	Project         string          `json:"project"`
	RepositoryRoot  string          `json:"repository_root"`
	ProjectRelative string          `json:"project_relative"`
	InitialCommit   string          `json:"initial_commit"`
	InventorySHA256 string          `json:"inventory_sha256"`
	Entries         []BaselineEntry `json:"entries"`
	CreatedAt       time.Time       `json:"created_at"`
}

type InitGitOptions struct {
	Project       string
	RequestFile   string
	AdoptExisting bool
}

func (s *Service) PlanGitBootstrap(ctx context.Context, options InitGitOptions) (GitBootstrapPlan, error) {
	if _, err := s.deps.LookPath("git"); err != nil {
		return GitBootstrapPlan{}, fmt.Errorf("git executable is unavailable: %w", err)
	}
	project, err := canonicalDirectory(options.Project)
	if err != nil {
		return GitBootstrapPlan{}, err
	}
	class, err := s.classifyRepository(ctx, project)
	if err != nil {
		return GitBootstrapPlan{}, err
	}
	if class == RepositoryUnsafeBoundary {
		return GitBootstrapPlan{}, errors.New("unsafe repository boundary: linked worktrees and submodules are not supported")
	}
	entries, err := baselineInventory(project)
	if err != nil {
		return GitBootstrapPlan{}, err
	}
	if class != RepositoryExisting {
		managed, managedErr := isManagedMinimal(project, options.RequestFile, entries)
		if managedErr != nil {
			return GitBootstrapPlan{}, managedErr
		}
		if class == RepositoryUnborn {
			if !managed && !options.AdoptExisting {
				return GitBootstrapPlan{}, errors.New("existing project files require --adopt-existing")
			}
		} else if managed {
			class = RepositoryManagedMinimal
		} else {
			class = RepositoryUnmanaged
			if !options.AdoptExisting {
				return GitBootstrapPlan{}, errors.New("existing project files require --adopt-existing")
			}
		}
	}
	ignoreBefore, ignoreAfter, err := plannedGitignore(project)
	if err != nil {
		return GitBootstrapPlan{}, err
	}
	digest, err := inventoryDigest(entries)
	if err != nil {
		return GitBootstrapPlan{}, err
	}
	idInput := strings.Join([]string{project, class, digest, ignoreAfter}, "\x00")
	idHash := sha256.Sum256([]byte(idInput))
	return GitBootstrapPlan{
		SchemaVersion: "1.0", ID: "bootstrap_" + hex.EncodeToString(idHash[:8]), Project: project,
		RepositoryClass: class, AdoptExisting: options.AdoptExisting, Inventory: entries,
		InventorySHA256: digest, GitignoreBefore: ignoreBefore, GitignoreAfter: ignoreAfter,
		CommitMessage: defaultBootstrapCommitMsg, CreatedAt: s.deps.Now().UTC(),
	}, nil
}

func (s *Service) ApplyGitBootstrap(ctx context.Context, plan GitBootstrapPlan) (GitBootstrapReceipt, error) {
	store := filesystem.New(plan.Project)
	lease, err := store.AcquireProjectWriterLease()
	if err != nil {
		return GitBootstrapReceipt{}, err
	}
	defer lease.Close() //nolint:errcheck

	current, err := s.PlanGitBootstrap(ctx, InitGitOptions{
		Project: plan.Project, RequestFile: filepath.Join(plan.Project, managedRequestPath(plan.Inventory)), AdoptExisting: plan.AdoptExisting,
	})
	if err != nil {
		return GitBootstrapReceipt{}, fmt.Errorf("revalidate bootstrap plan: %w", err)
	}
	if current.RepositoryClass != plan.RepositoryClass || current.InventorySHA256 != plan.InventorySHA256 ||
		current.GitignoreBefore != plan.GitignoreBefore || current.GitignoreAfter != plan.GitignoreAfter {
		return GitBootstrapReceipt{}, errors.New("bootstrap inventory changed after planning; create a new plan")
	}
	if plan.RepositoryClass == RepositoryExisting {
		return s.currentRepositoryReceipt(ctx, plan)
	}
	if err := s.requireGitAuthor(ctx, plan.Project); err != nil {
		return GitBootstrapReceipt{}, err
	}
	if err := writeRootGitignore(plan.Project); err != nil {
		return GitBootstrapReceipt{}, err
	}
	if plan.RepositoryClass != RepositoryUnborn {
		if _, err := s.gitText(ctx, plan.Project, "init"); err != nil {
			return GitBootstrapReceipt{}, fmt.Errorf("initialize git repository: %w", err)
		}
	}
	paths := make([]string, 0, len(plan.Inventory)+1)
	for _, entry := range plan.Inventory {
		paths = append(paths, entry.Path)
	}
	if !containsString(paths, ".gitignore") {
		paths = append(paths, ".gitignore")
	}
	sort.Strings(paths)
	addArgs := append([]string{"add", "--"}, paths...)
	if _, err := s.gitText(ctx, plan.Project, addArgs...); err != nil {
		return GitBootstrapReceipt{}, fmt.Errorf("stage bootstrap inventory: %w", err)
	}
	hooks := filepath.Join(plan.Project, ".rct", "bootstrap-hooks")
	if err := os.MkdirAll(hooks, 0o700); err != nil {
		return GitBootstrapReceipt{}, fmt.Errorf("create isolated hooks directory: %w", err)
	}
	if _, err := s.gitText(ctx, plan.Project,
		"-c", "core.hooksPath="+hooks, "-c", "commit.gpgsign=false",
		"commit", "--no-verify", "-m", plan.CommitMessage,
	); err != nil {
		return GitBootstrapReceipt{}, fmt.Errorf("create bootstrap commit: %w", err)
	}
	receipt, err := s.currentRepositoryReceipt(ctx, plan)
	if err != nil {
		return GitBootstrapReceipt{}, err
	}
	if err := s.requireCleanImplementationWorktree(ctx, plan.Project); err != nil {
		return GitBootstrapReceipt{}, fmt.Errorf("verify bootstrap worktree: %w", err)
	}
	encoded, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return GitBootstrapReceipt{}, err
	}
	if err := writeAtomicFile(filepath.Join(plan.Project, ".rct", "git-bootstrap-receipt.json"), append(encoded, '\n'), 0o600); err != nil {
		return GitBootstrapReceipt{}, fmt.Errorf("write bootstrap receipt: %w", err)
	}
	return receipt, nil
}

func (s *Service) InitGit(ctx context.Context, options InitGitOptions) (GitBootstrapPlan, GitBootstrapReceipt, error) {
	plan, err := s.PlanGitBootstrap(ctx, options)
	if err != nil {
		return GitBootstrapPlan{}, GitBootstrapReceipt{}, err
	}
	receipt, err := s.ApplyGitBootstrap(ctx, plan)
	return plan, receipt, err
}

func (s *Service) currentRepositoryReceipt(ctx context.Context, plan GitBootstrapPlan) (GitBootstrapReceipt, error) {
	root, err := s.gitText(ctx, plan.Project, "rev-parse", "--show-toplevel")
	if err != nil {
		return GitBootstrapReceipt{}, fmt.Errorf("read repository root: %w", err)
	}
	head, err := s.gitText(ctx, plan.Project, "rev-parse", "HEAD")
	if err != nil {
		return GitBootstrapReceipt{}, fmt.Errorf("read repository baseline: %w", err)
	}
	root = strings.TrimSpace(root)
	relative, err := filepath.Rel(root, plan.Project)
	if err != nil {
		return GitBootstrapReceipt{}, err
	}
	return GitBootstrapReceipt{
		SchemaVersion: "1.0", PlanID: plan.ID, Project: plan.Project, RepositoryRoot: root,
		ProjectRelative: filepath.ToSlash(relative), InitialCommit: strings.TrimSpace(head),
		InventorySHA256: plan.InventorySHA256, Entries: plan.Inventory, CreatedAt: s.deps.Now().UTC(),
	}, nil
}

func (s *Service) requireGitAuthor(ctx context.Context, project string) error {
	value, err := s.gitText(ctx, project, "var", "GIT_AUTHOR_IDENT")
	if err != nil || strings.TrimSpace(value) == "" {
		return errors.New("git author identity is missing: configure user.name and user.email before bootstrap")
	}
	return nil
}

func canonicalDirectory(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve project path: %w", err)
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("canonicalize project path: %w", err)
	}
	info, err := os.Stat(canonical)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("project path is not a directory: %s", absolute)
	}
	return canonical, nil
}

func (s *Service) classifyRepository(ctx context.Context, project string) (string, error) {
	gitPath := filepath.Join(project, ".git")
	if info, err := os.Lstat(gitPath); err == nil && !info.IsDir() {
		return RepositoryUnsafeBoundary, nil
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	root, err := s.gitText(ctx, project, "rev-parse", "--show-toplevel")
	if err != nil {
		return RepositoryUnmanaged, nil
	}
	if _, err := s.gitText(ctx, project, "rev-parse", "HEAD"); err != nil {
		return RepositoryUnborn, nil
	}
	if strings.TrimSpace(root) == "" {
		return RepositoryUnsafeBoundary, nil
	}
	index, err := s.gitText(ctx, project, "ls-files", "--stage")
	if err != nil {
		return "", fmt.Errorf("inspect repository index: %w", err)
	}
	for _, line := range strings.Split(index, "\n") {
		if strings.HasPrefix(line, "160000 ") {
			return RepositoryUnsafeBoundary, nil
		}
	}
	return RepositoryExisting, nil
}

func baselineInventory(project string) ([]BaselineEntry, error) {
	var entries []BaselineEntry
	var totalBytes int64
	err := filepath.WalkDir(project, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(project, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		first := strings.Split(filepath.ToSlash(relative), "/")[0]
		if entry.Name() == ".git" && first != ".git" {
			return fmt.Errorf("unsafe nested Git repository at %q", relative)
		}
		if first == ".git" || first == ".rct" {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
			return fmt.Errorf("unsupported bootstrap entry %q", relative)
		}
		if info.Mode().IsRegular() && info.Size() > maxBootstrapFileBytes {
			return fmt.Errorf("bootstrap entry %q exceeds the %d byte limit", relative, maxBootstrapFileBytes)
		}
		totalBytes += info.Size()
		if totalBytes > maxBootstrapTotalBytes {
			return fmt.Errorf("bootstrap inventory exceeds the %d byte limit", maxBootstrapTotalBytes)
		}
		var content []byte
		kind := "regular_file"
		gitMode := "100644"
		if info.Mode()&os.ModeSymlink != 0 {
			kind = "symlink"
			gitMode = "120000"
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			content = []byte(target)
		} else {
			if info.Mode().Perm()&0o111 != 0 {
				gitMode = "100755"
			}
			content, err = os.ReadFile(path)
			if err != nil {
				return err
			}
		}
		hash := sha256.Sum256(content)
		entries = append(entries, BaselineEntry{
			Path: filepath.ToSlash(relative), Kind: kind, Size: info.Size(), SHA256: hex.EncodeToString(hash[:]),
			Mode: gitMode,
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("inventory project: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries, nil
}

func isManagedMinimal(project, requestFile string, entries []BaselineEntry) (bool, error) {
	requestFile = strings.TrimSpace(requestFile)
	if requestFile == "" {
		return false, nil
	}
	absolute, err := filepath.Abs(requestFile)
	if err != nil {
		return false, err
	}
	absolute, err = filepath.EvalSymlinks(absolute)
	if err != nil {
		return false, fmt.Errorf("canonicalize request file: %w", err)
	}
	relative, err := filepath.Rel(project, absolute)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return false, errors.New("request file must be inside the project directory")
	}
	relative = filepath.ToSlash(relative)
	for _, entry := range entries {
		if entry.Path != relative && entry.Path != ".gitignore" {
			return false, nil
		}
		if entry.Kind != "regular_file" {
			return false, nil
		}
	}
	return containsEntry(entries, relative), nil
}

func plannedGitignore(project string) (string, string, error) {
	path := filepath.Join(project, ".gitignore")
	var before []byte
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() {
			return "", "", errors.New(".gitignore must be a regular file")
		}
		before, err = os.ReadFile(path)
		if err != nil {
			return "", "", err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", "", err
	}
	after := gitignoreWithRCT(before)
	return hashBytes(before), hashBytes(after), nil
}

func writeRootGitignore(project string) error {
	path := filepath.Join(project, ".gitignore")
	var before []byte
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() {
			return errors.New(".gitignore must be a regular file")
		}
		before, err = os.ReadFile(path)
		if err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return writeAtomicFile(path, gitignoreWithRCT(before), 0o644)
}

func gitignoreWithRCT(before []byte) []byte {
	for _, line := range strings.Split(strings.ReplaceAll(string(before), "\r\n", "\n"), "\n") {
		if strings.TrimSpace(line) == "/.rct/" {
			return before
		}
	}
	result := append([]byte(nil), before...)
	if len(result) > 0 && result[len(result)-1] != '\n' {
		result = append(result, '\n')
	}
	return append(result, []byte("/.rct/\n")...)
}

func writeAtomicFile(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".rct-write-*")
	if err != nil {
		return err
	}
	temp := file.Name()
	defer os.Remove(temp) //nolint:errcheck
	if err := file.Chmod(mode); err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(temp, path)
}

func inventoryDigest(entries []BaselineEntry) (string, error) {
	data, err := json.Marshal(entries)
	if err != nil {
		return "", err
	}
	return hashBytes(data), nil
}

func hashBytes(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func containsEntry(entries []BaselineEntry, path string) bool {
	for _, entry := range entries {
		if entry.Path == path {
			return true
		}
	}
	return false
}

func containsString(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func managedRequestPath(entries []BaselineEntry) string {
	for _, entry := range entries {
		if entry.Path != ".gitignore" {
			return entry.Path
		}
	}
	return ""
}
