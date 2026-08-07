package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hironeko/rct/internal/domain"
	"github.com/hironeko/rct/internal/store/filesystem"
)

func TestGitBootstrapManagedMinimalCreatesCleanBaseline(t *testing.T) {
	setTestGitIdentity(t)
	project := t.TempDir()
	requestPath := filepath.Join(project, "request.md")
	if err := os.WriteFile(requestPath, []byte("Build a small app\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	service := NewService(DefaultDependencies())
	plan, err := service.PlanGitBootstrap(context.Background(), InitGitOptions{
		Project: project, RequestFile: requestPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.RepositoryClass != RepositoryManagedMinimal {
		t.Fatalf("repository class = %q", plan.RepositoryClass)
	}
	receipt, err := service.ApplyGitBootstrap(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	canonicalProject, err := filepath.EvalSymlinks(project)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.InitialCommit == "" || receipt.RepositoryRoot != canonicalProject {
		t.Fatalf("receipt = %#v", receipt)
	}
	ignore, err := os.ReadFile(filepath.Join(project, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if string(ignore) != "/.rct/\n" {
		t.Fatalf(".gitignore = %q", ignore)
	}
	status := gitOutput(t, project, "status", "--porcelain=v1", "--untracked-files=all")
	if strings.TrimSpace(status) != "" {
		t.Fatalf("worktree is dirty: %q", status)
	}
}

func TestPreflightWaitsForBootstrapAndResumePreservesRun(t *testing.T) {
	setTestGitIdentity(t)
	project := t.TempDir()
	requestPath := filepath.Join(project, "request.md")
	if err := os.WriteFile(requestPath, []byte("Build it\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	service := NewService(Dependencies{LookPath: exec.LookPath})
	run, err := service.Start(context.Background(), StartOptions{
		Request: "Build it", Project: project, Mode: "supervised", Backend: "direct", SkipToolCheck: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	run = persistApprovedPlanFixture(t, run)
	waiting, err := service.ExecuteImplementationPreflight(context.Background(), run)
	if err != nil {
		t.Fatal(err)
	}
	if waiting.State != domain.StateWaitingForHuman || waiting.Interruption == nil ||
		waiting.Interruption.Code != InterruptionGitBootstrapRequired {
		t.Fatalf("waiting run = state %q interruption %#v", waiting.State, waiting.Interruption)
	}
	if _, _, err := service.InitGit(context.Background(), InitGitOptions{Project: project, RequestFile: requestPath}); err != nil {
		t.Fatal(err)
	}
	resumed, err := service.Resume(context.Background(), ResumeOptions{Project: project, RunID: run.ID})
	if err != nil {
		t.Fatal(err)
	}
	if resumed.ID != run.ID || resumed.State != domain.StateAwaitingApproval || resumed.BaseCommit == "" {
		t.Fatalf("resumed run = id %q state %q base %q", resumed.ID, resumed.State, resumed.BaseCommit)
	}
}

func TestPreflightDirtyWorktreeIsRecoverable(t *testing.T) {
	setTestGitIdentity(t)
	project := t.TempDir()
	gitOutput(t, project, "init")
	gitOutput(t, project, "config", "user.name", "RCT Test")
	gitOutput(t, project, "config", "user.email", "rct@example.invalid")
	if err := os.WriteFile(filepath.Join(project, "tracked.txt"), []byte("base\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitOutput(t, project, "add", "--", "tracked.txt")
	gitOutput(t, project, "commit", "--no-verify", "-m", "base")

	service := NewService(DefaultDependencies())
	run, err := service.Start(context.Background(), StartOptions{
		Request: "Build it", Project: project, Mode: "supervised", Backend: "direct", SkipToolCheck: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	run = persistApprovedPlanFixture(t, run)
	if err := os.WriteFile(filepath.Join(project, "tracked.txt"), []byte("dirty\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	waiting, err := service.ExecuteImplementationPreflight(context.Background(), run)
	if err != nil {
		t.Fatal(err)
	}
	if waiting.State != domain.StateWaitingForHuman || waiting.Interruption.Code != InterruptionDirtyWorktree || waiting.Failure != "" {
		t.Fatalf("dirty preflight = state %q interruption %#v failure %q", waiting.State, waiting.Interruption, waiting.Failure)
	}
}

func persistApprovedPlanFixture(t *testing.T, run domain.Run) domain.Run {
	t.Helper()
	store := filesystem.New(run.Project)
	plan := withTrailingNewline(testPlan(t))
	planPath, err := store.WriteRunFile(run.ID, "artifacts/plan/v001.json", plan)
	if err != nil {
		t.Fatal(err)
	}
	run.PlanPath = planPath
	run.PlanReview = filepath.ToSlash(filepath.Join(".rct", "runs", run.ID, "reviews", "plan-v001.json"))
	run.PlanSHA256 = sha256Hex(plan)
	run.ApprovalTargetHash = run.PlanSHA256
	run.LastVerdict = domain.VerdictApproved
	previous := run.State
	run.State = domain.StatePlanApproved
	run.Revision++
	if err := store.Update(run, previous, "PlanApproved"); err != nil {
		t.Fatal(err)
	}
	return run
}

func setTestGitIdentity(t *testing.T) {
	t.Helper()
	t.Setenv("GIT_AUTHOR_NAME", "RCT Test")
	t.Setenv("GIT_AUTHOR_EMAIL", "rct@example.invalid")
	t.Setenv("GIT_COMMITTER_NAME", "RCT Test")
	t.Setenv("GIT_COMMITTER_EMAIL", "rct@example.invalid")
}

func gitOutput(t *testing.T, project string, args ...string) string {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = project
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, output)
	}
	return string(output)
}
