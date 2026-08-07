package controlplane

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/hironeko/rct/internal/domain"
	"github.com/hironeko/rct/internal/store/filesystem"
)

const (
	maxCatalogDepth       = 8
	maxCatalogDirectories = 10000
)

type LocatedRun struct {
	Run     domain.Run
	Project string
}

type Catalog struct {
	roots       []string
	mu          sync.Mutex
	runs        map[string]LocatedRun
	lastScanned time.Time
}

func NewCatalog(roots []string) (*Catalog, error) {
	if len(roots) == 0 {
		return nil, errors.New("at least one workspace root is required")
	}
	canonical := make([]string, 0, len(roots))
	seen := map[string]bool{}
	for _, root := range roots {
		absolute, err := filepath.Abs(root)
		if err != nil {
			return nil, fmt.Errorf("resolve workspace root: %w", err)
		}
		resolved, err := filepath.EvalSymlinks(absolute)
		if err != nil {
			return nil, fmt.Errorf("canonicalize workspace root: %w", err)
		}
		info, err := os.Stat(resolved)
		if err != nil || !info.IsDir() {
			return nil, fmt.Errorf("workspace root is not a directory: %s", absolute)
		}
		if resolved == string(filepath.Separator) {
			return nil, errors.New("filesystem root cannot be used as a workspace root")
		}
		if !seen[resolved] {
			seen[resolved] = true
			canonical = append(canonical, resolved)
		}
	}
	return &Catalog{roots: canonical, runs: map[string]LocatedRun{}}, nil
}

func (c *Catalog) Resolve(runID string) (LocatedRun, error) {
	if err := c.refresh(false); err != nil {
		return LocatedRun{}, err
	}
	c.mu.Lock()
	located, ok := c.runs[runID]
	c.mu.Unlock()
	if ok {
		return located, nil
	}
	if err := c.refresh(true); err != nil {
		return LocatedRun{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	located, ok = c.runs[runID]
	if !ok {
		return LocatedRun{}, fmt.Errorf("run %q was not found in configured workspace roots", runID)
	}
	return located, nil
}

func (c *Catalog) List() ([]LocatedRun, error) {
	if err := c.refresh(false); err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([]LocatedRun, 0, len(c.runs))
	for _, run := range c.runs {
		result = append(result, run)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Run.UpdatedAt.After(result[j].Run.UpdatedAt)
	})
	return result, nil
}

func (c *Catalog) refresh(force bool) error {
	c.mu.Lock()
	if !force && time.Since(c.lastScanned) < 2*time.Second {
		c.mu.Unlock()
		return nil
	}
	c.mu.Unlock()

	discovered := map[string]LocatedRun{}
	for _, root := range c.roots {
		if err := scanRoot(root, discovered); err != nil {
			return err
		}
	}
	c.mu.Lock()
	c.runs = discovered
	c.lastScanned = time.Now()
	c.mu.Unlock()
	return nil
}

func scanRoot(root string, discovered map[string]LocatedRun) error {
	directories := 0
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if errors.Is(walkErr, fs.ErrPermission) {
				return filepath.SkipDir
			}
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !entry.IsDir() {
			return nil
		}
		directories++
		if directories > maxCatalogDirectories {
			return fmt.Errorf("workspace scan exceeded %d directories", maxCatalogDirectories)
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if relative != "." && len(strings.Split(relative, string(filepath.Separator))) > maxCatalogDepth {
			return filepath.SkipDir
		}
		name := entry.Name()
		if name == ".rct" {
			project := filepath.Dir(path)
			if err := scanRuns(project, discovered); err != nil {
				return err
			}
			return filepath.SkipDir
		}
		if relative != "." && (strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor") {
			return filepath.SkipDir
		}
		return nil
	})
}

func scanRuns(project string, discovered map[string]LocatedRun) error {
	runsDirectory := filepath.Join(project, ".rct", "runs")
	entries, err := os.ReadDir(runsDirectory)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read run catalog: %w", err)
	}
	store := filesystem.New(project)
	for _, entry := range entries {
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		run, err := store.Load(entry.Name())
		if err != nil {
			continue
		}
		if existing, exists := discovered[run.ID]; exists && existing.Project != project {
			return fmt.Errorf("duplicate run id %q exists in multiple projects", run.ID)
		}
		discovered[run.ID] = LocatedRun{Run: run, Project: project}
	}
	return nil
}
