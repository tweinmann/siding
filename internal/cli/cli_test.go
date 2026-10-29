package cli

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"

	"github.com/tweinmann/siding/internal/fail"

	"github.com/tweinmann/siding/internal/testutil"
)

var testBuild = BuildInfo{
	Version:   "v0.0.0-test",
	Commit:    "0123456789abcdef0123456789abcdef01234567",
	GoVersion: "go1.27.3",
	Platform:  "linux/arm64",
}

func run(args ...string) (code int, stdout, stderr string) {
	var out, errOut bytes.Buffer
	code = Execute(context.Background(), New(Options{Build: testBuild}), args, &out, &errOut)
	return code, out.String(), errOut.String()
}

// Every kind of document has one golden file in testdata/json.
func TestJSONDocuments(t *testing.T) {
	t.Parallel()
	tests := []struct {
		golden string
		args   []string
		code   int
	}{
		{golden: "version.json", args: []string{"version", "--json"}, code: exitOK},
		{golden: "error.json", args: []string{"version", "--json", "extra"}, code: exitFailed},
	}
	for _, tt := range tests {
		t.Run(tt.golden, func(t *testing.T) {
			t.Parallel()
			code, stdout, stderr := run(tt.args...)
			if code != tt.code {
				t.Errorf("exit code %d, want %d", code, tt.code)
			}
			if stderr != "" {
				t.Errorf("stderr is not empty: %q", stderr)
			}
			testutil.Golden(t, filepath.Join("testdata", "json", tt.golden), []byte(stdout))
		})
	}
}

func TestText(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		args   []string
		code   int
		stdout string
		stderr string
	}{
		{
			name:   "version",
			args:   []string{"version"},
			code:   exitOK,
			stdout: "siding v0.0.0-test (0123456789ab) go1.27.3 linux/arm64\n",
		},
		{
			name:   "unknown flag",
			args:   []string{"version", "--nope"},
			code:   exitFailed,
			stderr: "error: unknown flag: --nope\n",
		},
		{
			name:   "unknown command",
			args:   []string{"nope"},
			code:   exitFailed,
			stderr: "error: unknown command \"nope\" for \"siding\"\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			code, stdout, stderr := run(tt.args...)
			if code != tt.code || stdout != tt.stdout || stderr != tt.stderr {
				t.Errorf("got (%d, %q, %q), want (%d, %q, %q)", code, stdout, stderr, tt.code, tt.stdout, tt.stderr)
			}
		})
	}
}

// A flag error happens before --json is parsed; the document must still be JSON.
func TestErrorDocumentOnFlagError(t *testing.T) {
	t.Parallel()
	code, stdout, stderr := run("version", "--json", "--nope")
	if code != exitFailed || stderr != "" {
		t.Fatalf("got (%d, stderr %q), want (%d, no stderr)", code, stderr, exitFailed)
	}
	want := "{\n  \"apiVersion\": \"siding.dev/v1alpha1\",\n  \"kind\": \"Error\",\n  \"code\": \"InvalidArguments\",\n  \"message\": \"unknown flag: --nope\"\n}\n"
	if stdout != want {
		t.Errorf("stdout:\n%s\nwant:\n%s", stdout, want)
	}
}

// Every error code reaches the user the same way: one line and a hint on stderr, or an Error
// document with code, message and hint on stdout.
func TestErrorCodes(t *testing.T) {
	t.Parallel()
	codes := []string{
		fail.InvalidArguments, fail.NotAGitRepository, fail.NoBranch, fail.ComposeFileNotFound,
		fail.ComposeFailed, fail.DockerUnavailable, fail.ProxyNotRunning, fail.PortInUse,
		fail.InvalidRoute, fail.AmbiguousPort, fail.SlugCollision, fail.StackNotFound,
		fail.UnmergedBranch, fail.DirtyWorktree, fail.NoCapacity, fail.NotInteractive,
		fail.SnapshotNotFound, fail.SnapshotExists, fail.Internal,
	}
	for _, code := range codes {
		t.Run(code, func(t *testing.T) {
			t.Parallel()
			root := &cobra.Command{Use: "siding", SilenceUsage: true, SilenceErrors: true}
			var asJSON bool
			sub := &cobra.Command{Use: "boom", RunE: func(*cobra.Command, []string) error {
				return fail.New(code, "it broke").WithHint("try again")
			}}
			registerJSON(sub, &asJSON)
			root.AddCommand(sub)

			var out, errOut bytes.Buffer
			wantCode := exitFailed
			if code == fail.NoCapacity {
				wantCode = exitNoCapacity
			}
			if got := Execute(context.Background(), root, []string{"boom"}, &out, &errOut); got != wantCode {
				t.Errorf("exit code %d, want %d", got, wantCode)
			}
			if out.String() != "" || errOut.String() != "error: it broke\nhint: try again\n" {
				t.Errorf("text: stdout %q, stderr %q", out.String(), errOut.String())
			}

			out.Reset()
			errOut.Reset()
			root.SetArgs(nil)
			Execute(context.Background(), root, []string{"boom", "--json"}, &out, &errOut)
			want := "{\n  \"apiVersion\": \"siding.dev/v1alpha1\",\n  \"kind\": \"Error\",\n  \"code\": \"" + code +
				"\",\n  \"message\": \"it broke\",\n  \"hint\": \"try again\"\n}\n"
			if out.String() != want || errOut.String() != "" {
				t.Errorf("json: stdout %s, stderr %q", out.String(), errOut.String())
			}
		})
	}
}

// A command that printed a verdict ends with its exit code and nothing more.
func TestExitCodes(t *testing.T) {
	t.Parallel()
	for _, code := range []int{exitFailed, exitTimedOut, exitNoCapacity} {
		root := &cobra.Command{Use: "siding", SilenceUsage: true, SilenceErrors: true}
		root.AddCommand(&cobra.Command{Use: "verdict", RunE: func(*cobra.Command, []string) error { return exitCode(code) }})
		var out, errOut bytes.Buffer
		if got := Execute(context.Background(), root, []string{"verdict"}, &out, &errOut); got != code || out.Len()+errOut.Len() != 0 {
			t.Errorf("exit code %d (want %d), output %q %q", got, code, out.String(), errOut.String())
		}
	}
	// An error without a code is still reported in the same shape.
	root := &cobra.Command{Use: "siding", SilenceUsage: true, SilenceErrors: true}
	root.AddCommand(&cobra.Command{Use: "oops", RunE: func(*cobra.Command, []string) error { return errors.New("disk full") }})
	var out, errOut bytes.Buffer
	if got := Execute(context.Background(), root, []string{"oops"}, &out, &errOut); got != exitFailed || errOut.String() != "error: disk full\n" {
		t.Errorf("exit code %d, stderr %q", got, errOut.String())
	}
}

// Ctrl-C ends a command quietly with 130, e.g. logs --follow.
func TestInterrupted(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	root := &cobra.Command{Use: "siding", SilenceUsage: true, SilenceErrors: true}
	root.AddCommand(&cobra.Command{Use: "follow", RunE: func(cmd *cobra.Command, _ []string) error {
		cancel()
		<-cmd.Context().Done()
		return cmd.Context().Err()
	}})
	var out, errOut bytes.Buffer
	if got := Execute(ctx, root, []string{"follow"}, &out, &errOut); got != exitInterrupted || out.Len()+errOut.Len() != 0 {
		t.Errorf("exit code %d, output %q %q", got, out.String(), errOut.String())
	}
}
