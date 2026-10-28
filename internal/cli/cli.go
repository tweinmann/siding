// Package cli holds siding's commands: flags, arguments and output. What a command prints with
// --json is defined in json.go.
package cli

import (
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

// Exit codes. They are part of the contract with agents, like the --json documents: 0 Ready,
// 1 Failed, 2 TimedOut, 3 NoCapacity. A command that fails with an Error document exits 1.
const (
	exitOK     = 0
	exitFailed = 1
)

// Options holds everything a command needs from the machine it runs on. Tests pass their own,
// so no package-level variable is a test seam and tests can run in parallel.
type Options struct {
	Build BuildInfo
}

// New returns the root command.
func New(opts Options) *cobra.Command {
	root := &cobra.Command{
		Use:           "siding",
		Short:         "One Docker Compose stack per git worktree, and one view of all of them",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.CompletionOptions.DisableDefaultCmd = true
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return &Error{Code: CodeInvalidArguments, Message: err.Error()}
	})
	root.AddCommand(newVersionCommand(opts.Build))
	return root
}

// Execute runs root with args and returns the exit code. A failure is printed as an Error
// document on stdout when --json was given, and as one line on stderr otherwise.
func Execute(root *cobra.Command, args []string, stdout, stderr io.Writer) int {
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)
	cmd, err := root.ExecuteC()
	if err == nil {
		return exitOK
	}
	var e *Error
	if !errors.As(err, &e) {
		// Cobra reports an unknown command as a plain error.
		e = &Error{Code: CodeInvalidArguments, Message: err.Error()}
	}
	if wantsJSON(cmd, args) {
		if perr := printJSON(stdout, errorDocument{header: newHeader(kindError), Code: e.Code, Message: e.Message}); perr != nil {
			_, _ = fmt.Fprintf(stderr, "error: %s\n", e.Message)
		}
	} else {
		_, _ = fmt.Fprintf(stderr, "error: %s\n", e.Message)
	}
	return exitFailed
}

// Error is a failure with a stable code, which agents may branch on. Codes are never renamed
// within an apiVersion.
type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Message }

// Error codes.
const (
	// CodeInvalidArguments: unknown command, flag or argument.
	CodeInvalidArguments = "InvalidArguments"
)

// noArgs refuses positional arguments with CodeInvalidArguments.
func noArgs(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		return &Error{Code: CodeInvalidArguments, Message: fmt.Sprintf("%s takes no arguments, got %q", cmd.CommandPath(), args[0])}
	}
	return nil
}
