package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"

	"github.com/hironeko/rct/internal/controlplane"
	"github.com/hironeko/rct/web"
)

type repeatedStrings []string

func (values *repeatedStrings) String() string { return fmt.Sprint([]string(*values)) }
func (values *repeatedStrings) Set(value string) error {
	*values = append(*values, value)
	return nil
}

func (c *CLI) runServe(ctx context.Context, args []string) int {
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	flags.SetOutput(c.stderr)
	var workspaceRoots repeatedStrings
	flags.Var(&workspaceRoots, "workspace-root", "allowed workspace root; repeatable (default current directory)")
	listen := flags.String("listen", "127.0.0.1:0", "loopback listen address")
	open := flags.Bool("open", false, "open the browser after starting")
	noOpen := flags.Bool("no-open", false, "do not open the browser")
	asJSON := flags.Bool("json", false, "print server bootstrap information as JSON")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *open && *noOpen {
		fmt.Fprintln(c.stderr, "serve: --open and --no-open cannot be used together")
		return 2
	}
	if len(workspaceRoots) == 0 {
		current, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(c.stderr, "serve: resolve current directory: %v\n", err)
			return 1
		}
		workspaceRoots = append(workspaceRoots, current)
	}
	for index, root := range workspaceRoots {
		absolute, err := filepath.Abs(root)
		if err != nil {
			fmt.Fprintf(c.stderr, "serve: resolve workspace root: %v\n", err)
			return 1
		}
		workspaceRoots[index] = absolute
	}
	server, err := controlplane.NewServer(controlplane.Config{
		Listen: *listen, WorkspaceRoots: workspaceRoots, UI: web.Dist(),
		Approval: c.service, ApproverID: os.Getenv("USER"),
	})
	if err != nil {
		fmt.Fprintf(c.stderr, "serve: %v\n", err)
		return 1
	}
	bootstrap, err := server.Start()
	if err != nil {
		fmt.Fprintf(c.stderr, "serve: %v\n", err)
		return 1
	}
	shouldOpen := *open || (!*noOpen && isInteractive(c.stdout))
	if *asJSON {
		_ = json.NewEncoder(c.stdout).Encode(map[string]any{"url": bootstrap.URL, "listen": *listen})
	} else {
		fmt.Fprintf(c.stdout, "rct control plane: %s\n", bootstrap.URL)
		fmt.Fprintf(c.stdout, "Workspace roots: %d\n", len(workspaceRoots))
	}
	if shouldOpen {
		if err := openLocalBrowser(ctx, bootstrap.BootstrapURL); err != nil {
			if *open {
				_ = server.Close()
				fmt.Fprintf(c.stderr, "serve: open browser: %v\n", err)
				return 1
			}
			fmt.Fprintf(c.stderr, "rct: browser did not open automatically; open this one-time URL:\n%s\n", bootstrap.BootstrapURL)
		}
	} else {
		fmt.Fprintf(c.stderr, "Open this one-time local bootstrap URL:\n%s\n", bootstrap.BootstrapURL)
	}
	if err := server.Wait(ctx); err != nil {
		fmt.Fprintf(c.stderr, "serve: %v\n", err)
		return 1
	}
	return 0
}

func openLocalBrowser(ctx context.Context, target string) error {
	var executable string
	var args []string
	switch goruntime.GOOS {
	case "darwin":
		executable, args = "open", []string{target}
	case "linux":
		executable, args = "xdg-open", []string{target}
	default:
		return fmt.Errorf("automatic browser opening is unsupported on %s", goruntime.GOOS)
	}
	return exec.CommandContext(ctx, executable, args...).Run()
}
