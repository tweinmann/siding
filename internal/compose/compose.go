package compose

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/tweinmann/siding/internal/fail"
)

// MinVersion is the oldest Compose siding works with: `ports: !reset []` needs 2.24.
const MinVersion = "2.24"

// Invocation names a Compose project and the files it is made of.
type Invocation struct {
	// Dir is the working directory; Compose's project directory is the first file's.
	Dir     string
	Project string
	// Files are passed as -f, in order. Without files, Compose acts on the project by name.
	Files []string
}

func (inv Invocation) args(rest ...string) []string {
	args := []string{"compose", "-p", inv.Project}
	for _, f := range inv.Files {
		args = append(args, "-f", f)
	}
	return append(args, rest...)
}

// CLI runs docker compose. Its zero value uses "docker" from PATH.
type CLI struct {
	// Binary is the docker executable; empty means "docker".
	Binary string

	once       sync.Once
	versionErr error
}

func (c *CLI) bin() string {
	if c.Binary == "" {
		return "docker"
	}
	return c.Binary
}

// checkVersion fails with ComposeFailed unless Compose is installed and at least MinVersion.
// It runs once per CLI.
func (c *CLI) checkVersion(ctx context.Context) error {
	c.once.Do(func() {
		out, err := exec.CommandContext(ctx, c.bin(), "compose", "version", "--short").Output()
		if err != nil {
			c.versionErr = fail.New(fail.ComposeFailed, "docker compose is not available: %v", err).
				WithHint("install the Docker Compose plugin %s or later", MinVersion)
			return
		}
		v := strings.TrimPrefix(strings.TrimSpace(string(out)), "v")
		if !AtLeast(v, MinVersion) {
			c.versionErr = fail.New(fail.ComposeFailed, "docker compose %s is too old", v).
				WithHint("siding needs Docker Compose %s or later", MinVersion)
		}
	})
	return c.versionErr
}

// AtLeast compares dotted version numbers: AtLeast("2.24.1", "2.24") is true.
func AtLeast(have, want string) bool {
	h, w := strings.Split(have, "."), strings.Split(want, ".")
	for i := range w {
		var hn, wn int
		if i < len(h) {
			_, _ = fmt.Sscanf(h[i], "%d", &hn)
		}
		_, _ = fmt.Sscanf(w[i], "%d", &wn)
		if hn != wn {
			return hn > wn
		}
	}
	return true
}

// Config returns the output of docker compose config --format json.
func (c *CLI) Config(ctx context.Context, inv Invocation) ([]byte, error) {
	if err := c.checkVersion(ctx); err != nil {
		return nil, err
	}
	var out, errOut bytes.Buffer
	if err := c.exec(ctx, inv, inv.args("config", "--format", "json"), &out, &errOut); err != nil {
		return nil, composeError("read the compose configuration", errOut.String(), err)
	}
	return out.Bytes(), nil
}

// UnresolvedConfig prints the normalised configuration without reading env files, so it
// succeeds even when an env_file is missing; EnvFiles reads their paths from it.
func (c *CLI) UnresolvedConfig(ctx context.Context, inv Invocation) ([]byte, error) {
	if err := c.checkVersion(ctx); err != nil {
		return nil, err
	}
	var out, errOut bytes.Buffer
	if err := c.exec(ctx, inv, inv.args("config", "--no-env-resolution", "--format", "json"), &out, &errOut); err != nil {
		return nil, composeError("read the compose configuration", errOut.String(), err)
	}
	return out.Bytes(), nil
}

// Run runs docker compose with args, streaming its output. On failure the error carries the
// last lines Compose printed on stderr.
func (c *CLI) Run(ctx context.Context, inv Invocation, args []string, stdout, stderr io.Writer) error {
	if err := c.checkVersion(ctx); err != nil {
		return err
	}
	tail := &TailBuffer{Max: 20}
	if err := c.exec(ctx, inv, inv.args(args...), stdout, io.MultiWriter(stderr, tail)); err != nil {
		if ctx.Err() != nil {
			return ctx.Err() // interrupted, e.g. Ctrl-C during logs --follow
		}
		return composeError("docker compose "+strings.Join(args, " "), tail.String(), err)
	}
	return nil
}

func (c *CLI) exec(ctx context.Context, inv Invocation, args []string, stdout, stderr io.Writer) error {
	cmd := exec.CommandContext(ctx, c.bin(), args...)
	cmd.Dir = inv.Dir
	// No stdin: a question from Compose fails instead of waiting for an answer nobody gives.
	cmd.Stdin = nil
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

func composeError(what, output string, err error) error {
	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		return fail.New(fail.ComposeFailed, "%s: %v", what, err)
	}
	output = errorLines(output)
	if output == "" {
		return fail.New(fail.ComposeFailed, "%s failed with exit code %d", what, exit.ExitCode())
	}
	return fail.New(fail.ComposeFailed, "%s failed: %s", what, output)
}

// progress matches Compose's progress lines, such as "Container shop-db-1 Creating".
var progress = regexp.MustCompile(`^(Container|Network|Volume|Image)\s+\S+\s+\S+\s*$`)

// errorLines keeps what explains a failure in Compose's stderr: no progress lines, the lines
// that say "error" when there are any, each message once.
func errorLines(output string) string {
	var lines, errs []string
	seen := map[string]bool{}
	for _, l := range strings.Split(output, "\n") {
		l = strings.TrimSpace(l)
		if l == "" || progress.MatchString(l) {
			continue
		}
		// Compose repeats a service's error with a "service:<name>:<n> " prefix.
		if _, rest, ok := strings.Cut(l, " "); ok && strings.HasPrefix(l, "service:") {
			l = rest
		}
		if seen[l] {
			continue
		}
		seen[l] = true
		lines = append(lines, l)
		if strings.Contains(strings.ToLower(l), "error") {
			errs = append(errs, l)
		}
	}
	if len(errs) > 0 {
		lines = errs
	}
	return strings.Join(lines, "\n")
}

// TailBuffer keeps the last Max lines written to it.
type TailBuffer struct {
	Max     int
	lines   []string
	partial string
}

func (t *TailBuffer) Write(p []byte) (int, error) {
	s := t.partial + string(p)
	parts := strings.Split(s, "\n")
	t.partial = parts[len(parts)-1]
	t.lines = append(t.lines, parts[:len(parts)-1]...)
	if len(t.lines) > t.Max {
		t.lines = t.lines[len(t.lines)-t.Max:]
	}
	return len(p), nil
}

func (t *TailBuffer) String() string { return strings.Join(t.Lines(), "\n") }

// Lines returns the kept lines, the unfinished last one included.
func (t *TailBuffer) Lines() []string {
	lines := slices.Clone(t.lines)
	if t.partial != "" {
		lines = append(lines, t.partial)
	}
	if len(lines) > t.Max {
		lines = lines[len(lines)-t.Max:]
	}
	return lines
}
