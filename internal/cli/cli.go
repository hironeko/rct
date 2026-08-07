package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/hironeko/rct/internal/app"
	"github.com/hironeko/rct/internal/domain"
	"github.com/hironeko/rct/internal/store/filesystem"
)

var Version = "0.5.0-dev"

type CLI struct {
	service *app.Service
	stdin   io.Reader
	stdout  io.Writer
	stderr  io.Writer
}

func New(service *app.Service, stdin io.Reader, stdout, stderr io.Writer) *CLI {
	return &CLI{
		service: service,
		stdin:   stdin,
		stdout:  stdout,
		stderr:  stderr,
	}
}

func (c *CLI) Run(ctx context.Context, args []string) int {
	if len(args) == 0 {
		c.printUsage()
		return 2
	}

	switch args[0] {
	case "start":
		return c.runStart(ctx, args[1:])
	case "doctor":
		return c.runDoctor(ctx, args[1:])
	case "plan":
		return c.runPlan(ctx, args[1:])
	case "approve":
		return c.runApprove(ctx, args[1:])
	case "implement":
		return c.runImplement(ctx, args[1:])
	case "status":
		return c.runStatus(args[1:])
	case "watch":
		return c.runWatch(ctx, args[1:])
	case "serve":
		return c.runServe(ctx, args[1:])
	case "version":
		fmt.Fprintln(c.stdout, Version)
		return 0
	case "help", "-h", "--help":
		c.printUsage()
		return 0
	default:
		fmt.Fprintf(c.stderr, "unknown command %q\n", args[0])
		c.printUsage()
		return 2
	}
}

func (c *CLI) runStart(ctx context.Context, args []string) int {
	flags := flag.NewFlagSet("start", flag.ContinueOnError)
	flags.SetOutput(c.stderr)

	request := flags.String("request", "", "rough request to process")
	requestFile := flags.String("request-file", "", "file containing the rough request")
	mode := flags.String("mode", "supervised", "supervised, autonomous, or design-only")
	backend := flags.String("backend", "auto", "auto, herdr, tmux, or direct")
	designer := flags.String("designer", "", "designer provider: codex or claude")
	implementer := flags.String("implementer", "", "implementer provider: codex or claude")
	reviewer := flags.String("reviewer", "", "reviewer provider: codex or claude")
	project := flags.String("project", ".", "project directory")
	execute := flags.Bool("execute", false, "execute the design workflow after initializing the run")
	until := flags.String("until", "plan", "execute through requirements or plan")
	maxReviewRounds := flags.Int(
		"max-review-rounds",
		3,
		"maximum requirements review rounds before waiting for a human",
	)
	asJSON := flags.Bool("json", false, "print JSON")
	progress := flags.String("progress", "auto", "auto, tty, plain, jsonl, or none")
	notify := flags.String("notify", "auto", "auto, desktop, bell, or none")

	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *until != "requirements" && *until != "plan" {
		fmt.Fprintln(c.stderr, "start: --until must be requirements or plan")
		return 2
	}
	if err := validateLiveOptions(*progress, *notify); err != nil {
		fmt.Fprintf(c.stderr, "start: %v\n", err)
		return 2
	}

	resolvedRequest, err := c.resolveRequest(*request, *requestFile, flags.Args())
	if err != nil {
		fmt.Fprintf(c.stderr, "start: %v\n", err)
		return 2
	}

	run, err := c.service.Start(ctx, app.StartOptions{
		Request:     resolvedRequest,
		Project:     *project,
		Mode:        *mode,
		Backend:     *backend,
		Designer:    *designer,
		Implementer: *implementer,
		Reviewer:    *reviewer,
	})
	if err != nil {
		fmt.Fprintf(c.stderr, "start: %v\n", err)
		return 1
	}
	if *execute {
		observer := c.observeRun(ctx, filesystem.New(run.Project), run.ID, liveOptions{Progress: *progress, Notify: *notify, Writer: true})
		run, err = c.service.ExecuteDesign(ctx, run, *maxReviewRounds)
		if err != nil {
			observer.Stop()
			fmt.Fprintf(c.stderr, "start: execute design workflow: %v\n", err)
			return 1
		}
		if *until == "plan" && run.State == domain.StateRequirementsApproved {
			run, err = c.service.ExecutePlanning(ctx, run, *maxReviewRounds)
			if err != nil {
				observer.Stop()
				fmt.Fprintf(c.stderr, "start: execute planning workflow: %v\n", err)
				return 1
			}
		}
		observer.Stop()
	}

	if *asJSON {
		return c.writeJSON(run)
	}
	fmt.Fprintf(c.stdout, "Run initialized: %s\n", run.ID)
	fmt.Fprintf(c.stdout, "Project: %s\n", run.Project)
	fmt.Fprintf(c.stdout, "Backend: %s\n", run.Backend)
	fmt.Fprintf(c.stdout, "State: %s\n", run.State)
	fmt.Fprintf(
		c.stdout,
		"Roles: designer=%s implementer=%s reviewer=%s\n",
		run.Roles["designer"].Provider,
		run.Roles["implementer"].Provider,
		run.Roles["reviewer"].Provider,
	)
	if *execute {
		fmt.Fprintf(c.stdout, "Requirements rounds: %d\n", run.RequirementsRound)
		fmt.Fprintf(c.stdout, "Review verdict: %s\n", run.LastVerdict)
		if run.RequirementsPath != "" {
			fmt.Fprintf(c.stdout, "Requirements: %s\n", run.RequirementsPath)
		}
		if run.RequirementsReview != "" {
			fmt.Fprintf(c.stdout, "Review: %s\n", run.RequirementsReview)
		}
		if run.ArchitecturePath != "" {
			fmt.Fprintf(c.stdout, "Architecture: %s\n", run.ArchitecturePath)
		}
		if run.PlanPath != "" {
			fmt.Fprintf(c.stdout, "Plan: %s\n", run.PlanPath)
		}
	} else {
		fmt.Fprintln(
			c.stdout,
			"Run persisted at INTAKE. Add --execute --backend direct to run the design workflow.",
		)
	}
	return 0
}

func (c *CLI) runPlan(ctx context.Context, args []string) int {
	flags := flag.NewFlagSet("plan", flag.ContinueOnError)
	flags.SetOutput(c.stderr)
	project := flags.String("project", ".", "project directory")
	maxReviewRounds := flags.Int("max-review-rounds", 3, "maximum review rounds per planning artifact")
	asJSON := flags.Bool("json", false, "print JSON")
	progress := flags.String("progress", "auto", "auto, tty, plain, jsonl, or none")
	notify := flags.String("notify", "auto", "auto, desktop, bell, or none")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if err := validateLiveOptions(*progress, *notify); err != nil {
		fmt.Fprintf(c.stderr, "plan: %v\n", err)
		return 2
	}
	absoluteProject, err := filepath.Abs(*project)
	if err != nil {
		fmt.Fprintf(c.stderr, "plan: resolve project: %v\n", err)
		return 1
	}
	run, err := filesystem.New(absoluteProject).LoadCurrent()
	if err != nil {
		fmt.Fprintf(c.stderr, "plan: %v\n", err)
		return 1
	}
	observer := c.observeRun(ctx, filesystem.New(absoluteProject), run.ID, liveOptions{Progress: *progress, Notify: *notify, Writer: true})
	run, err = c.service.ExecutePlanning(ctx, run, *maxReviewRounds)
	observer.Stop()
	if err != nil {
		fmt.Fprintf(c.stderr, "plan: %v\n", err)
		return 1
	}
	if *asJSON {
		return c.writeJSON(run)
	}
	fmt.Fprintf(c.stdout, "Run: %s\n", run.ID)
	fmt.Fprintf(c.stdout, "State: %s\n", run.State)
	fmt.Fprintf(c.stdout, "Architecture: %s\n", run.ArchitecturePath)
	fmt.Fprintf(c.stdout, "Plan: %s\n", run.PlanPath)
	if run.State == domain.StateAwaitingApproval {
		fmt.Fprintln(c.stdout, "Next: rct approve --project <path>")
	}
	return 0
}

func (c *CLI) runApprove(ctx context.Context, args []string) int {
	flags := flag.NewFlagSet("approve", flag.ContinueOnError)
	flags.SetOutput(c.stderr)
	project := flags.String("project", ".", "project directory")
	runID := flags.String("run", "", "run id; defaults to the project current run")
	approverDefault := strings.TrimSpace(os.Getenv("USER"))
	if approverDefault == "" {
		approverDefault = "local-user"
	}
	approver := flags.String("by", approverDefault, "approver identifier")
	note := flags.String("note", "", "optional approval note")
	expectedRevision := flags.Uint64("revision", 0, "expected state revision; zero uses current revision")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	run, err := c.service.Approve(ctx, app.ApproveOptions{
		Project:          *project,
		RunID:            *runID,
		Approver:         *approver,
		Note:             *note,
		ExpectedRevision: *expectedRevision,
	})
	if err != nil {
		fmt.Fprintf(c.stderr, "approve: %v\n", err)
		return 1
	}
	if *asJSON {
		return c.writeJSON(run)
	}
	fmt.Fprintf(c.stdout, "Run: %s\n", run.ID)
	fmt.Fprintf(c.stdout, "State: %s\n", run.State)
	fmt.Fprintf(c.stdout, "Approved plan SHA-256: %s\n", run.Approval.SubjectSHA256)
	fmt.Fprintf(c.stdout, "Approval record: %s\n", run.ApprovalPath)
	fmt.Fprintln(c.stdout, "Next: rct implement --project <path>")
	return 0
}

func (c *CLI) runImplement(ctx context.Context, args []string) int {
	flags := flag.NewFlagSet("implement", flag.ContinueOnError)
	flags.SetOutput(c.stderr)
	project := flags.String("project", ".", "project directory")
	maxReviewRounds := flags.Int("max-review-rounds", 3, "maximum implementation review rounds per milestone")
	maxVerificationAttempts := flags.Int("max-verification-attempts", 3, "maximum verification attempts per milestone")
	asJSON := flags.Bool("json", false, "print JSON")
	progress := flags.String("progress", "auto", "auto, tty, plain, jsonl, or none")
	notify := flags.String("notify", "auto", "auto, desktop, bell, or none")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if err := validateLiveOptions(*progress, *notify); err != nil {
		fmt.Fprintf(c.stderr, "implement: %v\n", err)
		return 2
	}
	absoluteProject, err := filepath.Abs(*project)
	if err != nil {
		fmt.Fprintf(c.stderr, "implement: resolve project: %v\n", err)
		return 1
	}
	run, err := filesystem.New(absoluteProject).LoadCurrent()
	if err != nil {
		fmt.Fprintf(c.stderr, "implement: %v\n", err)
		return 1
	}
	observer := c.observeRun(ctx, filesystem.New(absoluteProject), run.ID, liveOptions{Progress: *progress, Notify: *notify, Writer: true})
	run, err = c.service.ExecuteImplementation(ctx, run, app.ImplementationOptions{
		MaxReviewRounds:         *maxReviewRounds,
		MaxVerificationAttempts: *maxVerificationAttempts,
	})
	observer.Stop()
	if err != nil {
		fmt.Fprintf(c.stderr, "implement: %v\n", err)
		return 1
	}
	if *asJSON {
		return c.writeJSON(run)
	}
	fmt.Fprintf(c.stdout, "Run: %s\n", run.ID)
	fmt.Fprintf(c.stdout, "State: %s\n", run.State)
	if len(run.CompletedMilestones) > 0 {
		fmt.Fprintf(c.stdout, "Completed milestones: %s\n", strings.Join(run.CompletedMilestones, ", "))
	}
	if run.WaitingReason != "" {
		fmt.Fprintf(c.stdout, "Waiting reason: %s\n", run.WaitingReason)
	}
	return 0
}

func (c *CLI) runDoctor(ctx context.Context, args []string) int {
	flags := flag.NewFlagSet("doctor", flag.ContinueOnError)
	flags.SetOutput(c.stderr)

	backend := flags.String("backend", "auto", "auto, herdr, tmux, or direct")
	designer := flags.String("designer", "", "designer provider: codex or claude")
	implementer := flags.String("implementer", "", "implementer provider: codex or claude")
	reviewer := flags.String("reviewer", "", "reviewer provider: codex or claude")
	project := flags.String("project", ".", "project directory")
	asJSON := flags.Bool("json", false, "print JSON")

	if err := flags.Parse(args); err != nil {
		return 2
	}

	report := c.service.Doctor(ctx, app.DoctorOptions{
		Project:     *project,
		Backend:     *backend,
		Designer:    *designer,
		Implementer: *implementer,
		Reviewer:    *reviewer,
	})
	if *asJSON {
		if code := c.writeJSON(report); code != 0 {
			return code
		}
	} else {
		for _, check := range report.Checks {
			marker := "OK"
			if !check.OK {
				if check.Required {
					marker = "FAIL"
				} else {
					marker = "INFO"
				}
			}
			fmt.Fprintf(c.stdout, "[%s] %s: %s\n", marker, check.Name, check.Detail)
		}
		if report.Backend != "" {
			fmt.Fprintf(c.stdout, "Selected backend: %s\n", report.Backend)
		}
	}
	if !report.Healthy {
		return 1
	}
	return 0
}

func (c *CLI) runStatus(args []string) int {
	flags := flag.NewFlagSet("status", flag.ContinueOnError)
	flags.SetOutput(c.stderr)
	project := flags.String("project", ".", "project directory")
	runID := flags.String("run", "", "run id; defaults to the project current run")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	absoluteProject, err := filepath.Abs(*project)
	if err != nil {
		fmt.Fprintf(c.stderr, "status: resolve project: %v\n", err)
		return 1
	}
	store := filesystem.New(absoluteProject)
	var run domain.Run
	if strings.TrimSpace(*runID) == "" {
		run, err = store.LoadCurrent()
	} else {
		run, err = store.Load(*runID)
	}
	if err != nil {
		fmt.Fprintf(c.stderr, "status: %v\n", err)
		return 1
	}
	snapshot, err := store.Progress(run.ID)
	if err != nil {
		fmt.Fprintf(c.stderr, "status: %v\n", err)
		return 1
	}
	if *asJSON {
		return c.writeJSON(snapshot)
	}
	fmt.Fprintf(c.stdout, "Run: %s\n", run.ID)
	fmt.Fprintf(c.stdout, "Project: %s\n", run.Project)
	fmt.Fprintf(c.stdout, "Backend: %s\n", run.Backend)
	fmt.Fprintf(c.stdout, "Mode: %s\n", run.Mode)
	fmt.Fprintf(c.stdout, "State: %s\n", run.State)
	if strings.TrimSpace(*runID) == "" {
		fmt.Fprintln(c.stdout, "Source: project current-run pointer")
	}
	if len(snapshot.Gauges) > 0 {
		gauge := snapshot.Gauges[0]
		fmt.Fprintf(c.stdout, "Overall progress: %d/%d %s\n", gauge.Completed, gauge.Total, gauge.Label)
	}
	if snapshot.Activity != nil {
		fmt.Fprintf(c.stdout, "Current activity: %s · %s · %s\n", phaseDisplay(snapshot.Activity.Phase), snapshot.Activity.Provider, snapshot.Activity.Action)
		fmt.Fprintf(c.stdout, "Job: %s\n", snapshot.Activity.JobID)
		if snapshot.Activity.Round > 0 {
			fmt.Fprintf(c.stdout, "Round budget: %d/%d\n", snapshot.Activity.Round, snapshot.Activity.MaxRounds)
		}
		fmt.Fprintf(c.stdout, "Liveness: %s\n", activityLiveness(snapshot.Activity))
	}
	if snapshot.NextAction != "" {
		fmt.Fprintf(c.stdout, "Next action: %s\n", snapshot.NextAction)
	}
	if run.RequirementsRound > 0 {
		fmt.Fprintf(c.stdout, "Requirements rounds: %d/%d\n", run.RequirementsRound, run.MaxReviewRounds)
	}
	if run.LastVerdict != "" {
		fmt.Fprintf(c.stdout, "Review verdict: %s\n", run.LastVerdict)
	}
	if run.RequirementsPath != "" {
		fmt.Fprintf(c.stdout, "Requirements: %s\n", run.RequirementsPath)
	}
	if run.RequirementsReview != "" {
		fmt.Fprintf(c.stdout, "Review: %s\n", run.RequirementsReview)
	}
	if run.ArchitectureRound > 0 {
		fmt.Fprintf(c.stdout, "Architecture rounds: %d/%d\n", run.ArchitectureRound, run.MaxReviewRounds)
	}
	if run.ArchitecturePath != "" {
		fmt.Fprintf(c.stdout, "Architecture: %s\n", run.ArchitecturePath)
	}
	if run.PlanRound > 0 {
		fmt.Fprintf(c.stdout, "Plan rounds: %d/%d\n", run.PlanRound, run.MaxReviewRounds)
	}
	if run.PlanPath != "" {
		fmt.Fprintf(c.stdout, "Plan: %s\n", run.PlanPath)
	}
	if run.WaitingReason != "" {
		fmt.Fprintf(c.stdout, "Waiting reason: %s\n", run.WaitingReason)
	}
	if run.ApprovalPath != "" {
		fmt.Fprintf(c.stdout, "Human approval: %s\n", run.ApprovalPath)
	}
	if run.CurrentMilestoneID != "" {
		fmt.Fprintf(c.stdout, "Current milestone: %s\n", run.CurrentMilestoneID)
		fmt.Fprintf(c.stdout, "Implementation round: %d/%d\n", run.ImplementationRound, run.MaxReviewRounds)
		fmt.Fprintf(c.stdout, "Verification attempts: %d\n", run.VerificationAttempts)
	}
	if len(run.CompletedMilestones) > 0 {
		fmt.Fprintf(c.stdout, "Completed milestones: %s\n", strings.Join(run.CompletedMilestones, ", "))
	}
	if run.VerificationPath != "" {
		fmt.Fprintf(c.stdout, "Verification: %s\n", run.VerificationPath)
	}
	if run.CodeReviewPath != "" {
		fmt.Fprintf(c.stdout, "Code review: %s\n", run.CodeReviewPath)
	}
	if run.Failure != "" {
		fmt.Fprintf(c.stdout, "Failure: %s\n", run.Failure)
	}
	return 0
}

func (c *CLI) runWatch(ctx context.Context, args []string) int {
	flags := flag.NewFlagSet("watch", flag.ContinueOnError)
	flags.SetOutput(c.stderr)
	project := flags.String("project", ".", "project directory")
	runID := flags.String("run", "", "run id; defaults to the project current run")
	follow := flags.Bool("follow", false, "follow changes until the run needs attention or finishes")
	format := flags.String("format", "plain", "plain or jsonl")
	notify := flags.String("notify", "none", "auto, desktop, bell, or none")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if !oneOf(*format, "plain", "jsonl") {
		fmt.Fprintln(c.stderr, "watch: --format must be plain or jsonl")
		return 2
	}
	if !oneOf(*notify, "auto", "desktop", "bell", "none") {
		fmt.Fprintln(c.stderr, "watch: --notify must be auto, desktop, bell, or none")
		return 2
	}
	absoluteProject, err := filepath.Abs(*project)
	if err != nil {
		fmt.Fprintf(c.stderr, "watch: resolve project: %v\n", err)
		return 1
	}
	store := filesystem.New(absoluteProject)
	var run domain.Run
	if strings.TrimSpace(*runID) == "" {
		run, err = store.LoadCurrent()
	} else {
		run, err = store.Load(*runID)
	}
	if err != nil {
		fmt.Fprintf(c.stderr, "watch: %v\n", err)
		return 1
	}
	if !*follow {
		snapshot, progressErr := store.Progress(run.ID)
		if progressErr != nil {
			fmt.Fprintf(c.stderr, "watch: %v\n", progressErr)
			return 1
		}
		if *format == "jsonl" {
			renderJSONLine(c.stdout, map[string]any{"kind": "snapshot", "snapshot": snapshot})
		} else {
			renderPlainSnapshot(c.stdout, snapshot)
		}
		return 0
	}
	stop := make(chan struct{})
	c.runObserver(ctx, store, run.ID, *format, *notify, stop, true, true)
	return 0
}

func (c *CLI) resolveRequest(direct, file string, positional []string) (string, error) {
	sources := 0
	if strings.TrimSpace(direct) != "" {
		sources++
	}
	if strings.TrimSpace(file) != "" {
		sources++
	}
	if len(positional) > 0 {
		sources++
	}
	if sources > 1 {
		return "", errors.New("use only one of --request, --request-file, or a positional request")
	}
	if strings.TrimSpace(file) != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			return "", fmt.Errorf("read request file: %w", err)
		}
		return requireRequest(string(data))
	}
	if strings.TrimSpace(direct) != "" {
		return requireRequest(direct)
	}
	if len(positional) > 0 {
		return requireRequest(strings.Join(positional, " "))
	}

	if input, ok := c.stdin.(*os.File); ok {
		info, err := input.Stat()
		if err == nil && info.Mode()&os.ModeCharDevice != 0 {
			return "", errors.New("request is required")
		}
	}
	data, err := io.ReadAll(c.stdin)
	if err != nil {
		return "", fmt.Errorf("read request from stdin: %w", err)
	}
	return requireRequest(string(data))
}

func requireRequest(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("request is required")
	}
	return value, nil
}

func (c *CLI) writeJSON(value any) int {
	encoder := json.NewEncoder(c.stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		fmt.Fprintf(c.stderr, "encode JSON: %v\n", err)
		return 1
	}
	return 0
}

func (c *CLI) printUsage() {
	fmt.Fprintln(c.stderr, "Usage: rct <command> [options]")
	fmt.Fprintln(c.stderr, "Commands: start, plan, approve, implement, doctor, status, watch, serve, version")
}
