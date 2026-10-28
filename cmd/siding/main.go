// Command siding gives every git worktree its own Docker Compose stack, and one view of all of
// them.
package main

import (
	"os"

	"github.com/tweinmann/siding/internal/cli"
)

// version is set at build time with -ldflags "-X main.version=<tag>".
var version = "dev"

func main() {
	os.Exit(cli.Execute(cli.New(cli.Options{Build: cli.CurrentBuild(version)}), os.Args[1:], os.Stdout, os.Stderr))
}
