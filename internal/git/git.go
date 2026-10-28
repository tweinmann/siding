// Package git answers what siding needs to know about a worktree by running the git CLI: where
// it is, which branch it is on, and whether removing its stack's data could lose work.
package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/tweinmann/siding/internal/fail"
)

// Worktree is a git worktree.
type Worktree struct {
	// Top is the absolute path of the worktree's top-level directory.
	Top string
	// CommonDir is the absolute path of the repository's common git directory, the same for
	// every worktree of the repository.
	CommonDir string
	// Branch is the checked-out branch, without refs/heads/.
	Branch string
}

// RepoName is the name of the directory holding the common git directory: the main worktree's
// directory, the same for every worktree.
func (w Worktree) RepoName() string {
	return filepath.Base(filepath.Dir(w.CommonDir))
}

// Client runs git.
type Client struct {
	// Binary is the git executable; empty means "git" from PATH.
	Binary string
}

func (c Client) run(ctx context.Context, dir string, args ...string) (string, error) {
	bin := c.Binary
	if bin == "" {
		bin = "git"
	}
	cmd := exec.CommandContext(ctx, bin, append([]string{"-C", dir}, args...)...)
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	err := cmd.Run()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return out.String(), &exitError{code: exit.ExitCode(), stderr: strings.TrimSpace(errOut.String())}
		}
		return "", fmt.Errorf("run git: %w", err)
	}
	return out.String(), nil
}

type exitError struct {
	code   int
	stderr string
}

func (e *exitError) Error() string { return fmt.Sprintf("git exited %d: %s", e.code, e.stderr) }

func exitCode(err error) int {
	var e *exitError
	if errors.As(err, &e) {
		return e.code
	}
	return -1
}

// Worktree returns the worktree containing dir. It fails with NotAGitRepository outside a
// worktree and with NoBranch when HEAD is detached.
func (c Client) Worktree(ctx context.Context, dir string) (Worktree, error) {
	out, err := c.run(ctx, dir, "rev-parse", "--path-format=absolute", "--show-toplevel", "--git-common-dir")
	if err != nil {
		if exitCode(err) > 0 {
			return Worktree{}, fail.New(fail.NotAGitRepository, "%s is not inside a git worktree", dir).
				WithHint("run siding in a git worktree of the repository whose stack you want")
		}
		return Worktree{}, err
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		return Worktree{}, fail.New(fail.NotAGitRepository, "%s is not inside a git worktree", dir)
	}
	w := Worktree{Top: lines[0], CommonDir: lines[1]}
	branch, err := c.run(ctx, dir, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		if exitCode(err) > 0 {
			return Worktree{}, fail.New(fail.NoBranch, "HEAD of %s is detached", w.Top).
				WithHint("check out a branch: every stack belongs to one branch")
		}
		return Worktree{}, err
	}
	w.Branch = strings.TrimSpace(branch)
	return w, nil
}

// Dirty reports whether git status lists any change in the worktree, untracked files
// included and ignored files excluded.
func (c Client) Dirty(ctx context.Context, top string) (bool, error) {
	out, err := c.run(ctx, top, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

// DefaultBranch returns the ref the repository merges into: what origin/HEAD points to, else
// a local main, else a local master. It returns "" when there is none.
func (c Client) DefaultBranch(ctx context.Context, top string) (string, error) {
	if out, err := c.run(ctx, top, "symbolic-ref", "--quiet", "refs/remotes/origin/HEAD"); err == nil {
		return strings.TrimSpace(out), nil
	} else if exitCode(err) < 0 {
		return "", err
	}
	for _, ref := range []string{"refs/heads/main", "refs/heads/master"} {
		if _, err := c.run(ctx, top, "rev-parse", "--verify", "--quiet", ref); err == nil {
			return ref, nil
		} else if exitCode(err) < 0 {
			return "", err
		}
	}
	return "", nil
}

// Merged reports whether the tip of branch is contained in the default branch, and which ref
// that is. Without a default branch it reports false.
func (c Client) Merged(ctx context.Context, top, branch string) (merged bool, base string, err error) {
	base, err = c.DefaultBranch(ctx, top)
	if err != nil || base == "" {
		return false, "", err
	}
	_, err = c.run(ctx, top, "merge-base", "--is-ancestor", "refs/heads/"+branch, base)
	switch code := exitCode(err); {
	case err == nil:
		return true, base, nil
	case code == 1:
		return false, base, nil
	default:
		return false, base, err
	}
}
