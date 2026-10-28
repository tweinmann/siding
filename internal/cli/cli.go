// Package cli holds siding's commands: flags, arguments and output. What a command prints with
// --json is defined in json.go.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/tweinmann/siding/internal/compose"
	"github.com/tweinmann/siding/internal/docker"
	"github.com/tweinmann/siding/internal/fail"
	"github.com/tweinmann/siding/internal/git"
	"github.com/tweinmann/siding/internal/probe"
	"github.com/tweinmann/siding/internal/proxy"
)

// Exit codes. They are part of the contract with agents, like the --json documents: 0 Ready,
// 1 Failed, 2 TimedOut (or Starting without --wait), 3 NoCapacity. A command that fails with
// an Error document exits 1.
const (
	exitOK       = 0
	exitFailed   = 1
	exitTimedOut = 2
	// exitNoCapacity: up refused because the stack does not fit into the memory budget.
	exitNoCapacity = 3
	// exitInterrupted ends a command stopped by Ctrl-C or SIGTERM, without a message, as
	// shells do.
	exitInterrupted = 130
)

// Options holds everything a command needs from the machine it runs on. Tests pass their own,
// so no package-level variable is a test seam and tests can run in parallel.
type Options struct {
	Build BuildInfo
	// Dir is the current directory.
	Dir string
	// Home is the user's home directory, for the cache under ~/.siding.
	Home string
	// Getenv reads an environment variable.
	Getenv func(string) string
	// OSMachineID returns the operating system's machine id, or "".
	OSMachineID func() string
	// IsLocalAddress reports whether an IP address belongs to this machine.
	IsLocalAddress func(ip string) bool
	// Stdin is where questions are answered; IsTerminal reports whether a human is there.
	Stdin      io.Reader
	IsTerminal func() bool

	Git     Git
	Compose Compose
	Docker  Docker
	Probe   Prober
	Browser Browser
	Clock   Clock
	// Proxy names siding-proxy and its network.
	Proxy proxy.Config
}

// Git answers questions about worktrees.
type Git interface {
	Worktree(ctx context.Context, dir string) (git.Worktree, error)
	Dirty(ctx context.Context, top string) (bool, error)
	MergeState(ctx context.Context, r git.Repo, branch string) (git.MergeState, error)
}

// Compose runs docker compose.
type Compose interface {
	Config(ctx context.Context, inv compose.Invocation) ([]byte, error)
	Run(ctx context.Context, inv compose.Invocation, args []string, stdout, stderr io.Writer) error
}

// Docker reads state from the Docker Engine API.
type Docker interface {
	Ping(ctx context.Context) error
	Containers(ctx context.Context, labels map[string]string) ([]docker.Container, error)
	Container(ctx context.Context, name string) (*docker.Container, error)
	Networks(ctx context.Context, labels map[string]string) ([]docker.Network, error)
	Volumes(ctx context.Context, labels map[string]string) ([]docker.Volume, error)
	EnsureNetwork(ctx context.Context, name string, labels map[string]string) (bool, error)
	MemTotal(ctx context.Context) (int64, error)
	NetworkGateway(ctx context.Context, name string) (string, error)
	NetworkUsers(ctx context.Context, name string) (exists bool, users []string, err error)
	RemoveNetwork(ctx context.Context, name string) error
	Logs(ctx context.Context, id string, tail int) ([]string, error)
	Memory(ctx context.Context, id string) (uint64, error)
}

// Prober sends one request through the proxy.
type Prober interface {
	Get(ctx context.Context, addr, host, path string) (probe.Response, error)
}

// Browser opens a URL.
type Browser interface {
	Open(url string) error
}

// Clock is the time, for waiting.
type Clock interface {
	Now() time.Time
	Sleep(ctx context.Context, d time.Duration) error
}

// New returns the root command.
func New(opts Options) *cobra.Command {
	a := &app{opts: opts}
	root := &cobra.Command{
		Use:           "siding",
		Short:         "One Docker Compose stack per git worktree, and one view of all of them",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.CompletionOptions.DisableDefaultCmd = true
	// An error is one line (and a hint); cobra's suggestions would add several.
	root.DisableSuggestions = true
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return &Error{Code: CodeInvalidArguments, Message: err.Error()}
	})
	root.AddCommand(
		newVersionCommand(opts.Build),
		newSetupCommand(a),
		newUpCommand(a),
		newStatusCommand(a),
		newPsCommand(a),
		newLogsCommand(a),
		newOpenCommand(a),
		newDownCommand(a),
		newGCCommand(a),
		newLintCommand(a),
		newUICommand(a),
		newTeardownCommand(a),
		newSkillCommand(),
	)
	return root
}

// app gives commands the Options.
type app struct {
	opts Options
}

// Execute runs root with args and returns the exit code. A failure is printed as an Error
// document on stdout when --json was given, and as one line (and a hint) on stderr otherwise.
func Execute(ctx context.Context, root *cobra.Command, args []string, stdout, stderr io.Writer) int {
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)
	cmd, err := root.ExecuteContextC(ctx)
	if err == nil {
		return exitOK
	}
	var code exitCode
	if errors.As(err, &code) {
		return int(code)
	}
	if ctx.Err() != nil && errors.Is(err, ctx.Err()) {
		return exitInterrupted
	}
	var e *Error
	if !errors.As(err, &e) {
		// Cobra reports an unknown command as a plain error; anything else unexpected is
		// still reported in the same shape.
		e = &Error{Code: CodeInvalidArguments, Message: err.Error()}
		if cmd != nil && cmd != root {
			e.Code = CodeInternal
		}
	}
	if wantsJSON(cmd, args) {
		doc := errorDocument{header: newHeader(kindError), Code: e.Code, Message: e.Message, Hint: e.Hint}
		if perr := printJSON(stdout, doc); perr != nil {
			_, _ = fmt.Fprintf(stderr, "error: %s\n", e.Message)
		}
	} else {
		_, _ = fmt.Fprintf(stderr, "error: %s\n", e.Message)
		if e.Hint != "" {
			_, _ = fmt.Fprintf(stderr, "hint: %s\n", e.Hint)
		}
	}
	if e.Code == fail.NoCapacity {
		return exitNoCapacity
	}
	return exitFailed
}

// exitCode ends a command that has already printed its result with this exit code.
type exitCode int

func (c exitCode) Error() string { return fmt.Sprintf("exit code %d", int(c)) }

// Error is a failure with a stable code, which agents may branch on.
type Error = fail.Error

// Error codes Execute uses itself; the rest are in package fail.
const (
	// CodeInvalidArguments: unknown command, flag or argument.
	CodeInvalidArguments = fail.InvalidArguments
	// CodeInternal: an unexpected error without a code of its own.
	CodeInternal = fail.Internal
)

// noArgs refuses positional arguments with CodeInvalidArguments.
func noArgs(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		return &Error{Code: CodeInvalidArguments, Message: fmt.Sprintf("%s takes no arguments, got %q", cmd.CommandPath(), args[0])}
	}
	return nil
}

// SystemClock is the real time.
type SystemClock struct{}

// Now returns the current time.
func (SystemClock) Now() time.Time { return time.Now() }

// Sleep waits for d or until ctx is done.
func (SystemClock) Sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
