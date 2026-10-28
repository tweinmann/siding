// Command siding gives every git worktree its own Docker Compose stack, and one view of all of
// them.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/tweinmann/siding/internal/cli"
	"github.com/tweinmann/siding/internal/compose"
	"github.com/tweinmann/siding/internal/docker"
	"github.com/tweinmann/siding/internal/git"
	"github.com/tweinmann/siding/internal/probe"
	"github.com/tweinmann/siding/internal/proxy"
	"github.com/tweinmann/siding/internal/stack"
)

// version is set at build time with -ldflags "-X main.version=<tag>".
var version = "dev"

func main() {
	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	opts := cli.Options{
		Build:       cli.CurrentBuild(version),
		Dir:         dir,
		Home:        home,
		Getenv:      os.Getenv,
		OSMachineID: stack.OSMachineID,
		Git:         git.Client{},
		Compose:     &compose.CLI{},
		Docker:      &docker.Client{},
		Probe:       probe.HTTP{Header: stack.ServiceHeader},
		Browser:     cli.SystemBrowser{Getenv: os.Getenv},
		Clock:       cli.SystemClock{},
		Proxy:       proxy.Default(),
	}
	code := cli.Execute(ctx, cli.New(opts), os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}
