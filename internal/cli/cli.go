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

	"github.com/hironeko/loop-engine/internal/app"
	"github.com/hironeko/loop-engine/internal/store/filesystem"
)

const Version = "0.3.0-dev"

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
	case "status":
		return c.runStatus(args[1:])
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
	maxReviewRounds := flags.Int(
		"max-review-rounds",
		3,
		"maximum requirements review rounds before waiting for a human",
	)
	asJSON := flags.Bool("json", false, "print JSON")

	if err := flags.Parse(args); err != nil {
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
		run, err = c.service.ExecuteDesign(ctx, run, *maxReviewRounds)
		if err != nil {
			fmt.Fprintf(c.stderr, "start: execute design workflow: %v\n", err)
			return 1
		}
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
	} else {
		fmt.Fprintln(
			c.stdout,
			"Run persisted at INTAKE. Add --execute --backend direct to run the design workflow.",
		)
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
	asJSON := flags.Bool("json", false, "print JSON")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	absoluteProject, err := filepath.Abs(*project)
	if err != nil {
		fmt.Fprintf(c.stderr, "status: resolve project: %v\n", err)
		return 1
	}
	run, err := filesystem.New(absoluteProject).LoadCurrent()
	if err != nil {
		fmt.Fprintf(c.stderr, "status: %v\n", err)
		return 1
	}
	if *asJSON {
		return c.writeJSON(run)
	}
	fmt.Fprintf(c.stdout, "Run: %s\n", run.ID)
	fmt.Fprintf(c.stdout, "Project: %s\n", run.Project)
	fmt.Fprintf(c.stdout, "Backend: %s\n", run.Backend)
	fmt.Fprintf(c.stdout, "Mode: %s\n", run.Mode)
	fmt.Fprintf(c.stdout, "State: %s\n", run.State)
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
	if run.Failure != "" {
		fmt.Fprintf(c.stdout, "Failure: %s\n", run.Failure)
	}
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
	fmt.Fprintln(c.stderr, "Usage: loop-engine <command> [options]")
	fmt.Fprintln(c.stderr, "Commands: start, doctor, status, version")
}
