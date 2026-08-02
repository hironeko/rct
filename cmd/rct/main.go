package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/hironeko/rct/internal/app"
	"github.com/hironeko/rct/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	service := app.NewService(app.DefaultDependencies())
	command := cli.New(service, os.Stdin, os.Stdout, os.Stderr)
	os.Exit(command.Run(ctx, os.Args[1:]))
}
