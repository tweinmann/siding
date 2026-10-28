package compose

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
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

// Run runs docker compose with args, streaming its output. On failure the error carries the
// last lines Compose printed on stderr.
func (c *CLI) Run(ctx context.Context, inv Invocation, args []string, stdout, stderr io.Writer) error {
	if err := c.checkVersion(ctx); err != nil {
		return err
	}
	tail := &tailBuffer{max: 20}
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
	output = strings.TrimSpace(output)
	if output == "" {
		return fail.New(fail.ComposeFailed, "%s failed with exit code %d", what, exit.ExitCode())
	}
	return fail.New(fail.ComposeFailed, "%s failed: %s", what, output)
}

// tailBuffer keeps the last max lines written to it.
type tailBuffer struct {
	max     int
	lines   []string
	partial string
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	s := t.partial + string(p)
	parts := strings.Split(s, "\n")
	t.partial = parts[len(parts)-1]
	t.lines = append(t.lines, parts[:len(parts)-1]...)
	if len(t.lines) > t.max {
		t.lines = t.lines[len(t.lines)-t.max:]
	}
	return len(p), nil
}

func (t *tailBuffer) String() string {
	lines := slices.Clone(t.lines)
	if t.partial != "" {
		lines = append(lines, t.partial)
	}
	if len(lines) > t.max {
		lines = lines[len(lines)-t.max:]
	}
	return strings.Join(lines, "\n")
}
