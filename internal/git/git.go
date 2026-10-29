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

// MainTop is the main worktree's top-level directory: the one holding the common git directory
// when that is named .git. It is "" for a bare repository or a common directory elsewhere.
func (w Worktree) MainTop() string {
	if filepath.Base(w.CommonDir) != ".git" {
		return ""
	}
	return filepath.Dir(w.CommonDir)
}

// Linked reports whether w is a linked worktree of a repository with a main worktree.
func (w Worktree) Linked() bool {
	main := w.MainTop()
	return main != "" && main != w.Top
}

// Client runs git.
type Client struct {
	// Binary is the git executable; empty means "git" from PATH.
	Binary string
}

func (c Client) run(ctx context.Context, dir string, args ...string) (string, error) {
	return c.exec(ctx, append([]string{"-C", dir}, args...))
}

func (c Client) exec(ctx context.Context, args []string) (string, error) {
	bin := c.Binary
	if bin == "" {
		bin = "git"
	}
	cmd := exec.CommandContext(ctx, bin, args...)
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

// Top returns the top-level directory of the worktree containing dir, also with a detached
// HEAD. It fails with NotAGitRepository outside a worktree.
func (c Client) Top(ctx context.Context, dir string) (string, error) {
	out, err := c.run(ctx, dir, "rev-parse", "--path-format=absolute", "--show-toplevel")
	if err != nil {
		if exitCode(err) > 0 {
			return "", fail.New(fail.NotAGitRepository, "%s is not inside a git worktree", dir).
				WithHint("run it in a git worktree, or name the skills directory with --dir")
		}
		return "", err
	}
	return strings.TrimSpace(out), nil
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

// Tracked reports whether git tracks path, relative to the worktree top.
func (c Client) Tracked(ctx context.Context, top, path string) (bool, error) {
	_, err := c.run(ctx, top, "ls-files", "--error-unmatch", "--", path)
	switch {
	case err == nil:
		return true, nil
	case exitCode(err) == 1:
		return false, nil
	}
	return false, err
}

// Repo names a repository for commands that need no worktree: the worktree's directory, or,
// when the worktree is gone, the common git directory.
type Repo struct {
	Dir    string
	GitDir string
}

func (r Repo) args(rest ...string) []string {
	if r.GitDir != "" {
		return append([]string{"--git-dir=" + r.GitDir}, rest...)
	}
	return append([]string{"-C", r.Dir}, rest...)
}

func (c Client) git(ctx context.Context, r Repo, args ...string) (string, error) {
	return c.exec(ctx, r.args(args...))
}

// DefaultBranch returns the ref the repository merges into: what origin/HEAD points to, else
// a local main, else a local master. It returns "" when there is none.
func (c Client) DefaultBranch(ctx context.Context, r Repo) (string, error) {
	if out, err := c.git(ctx, r, "symbolic-ref", "--quiet", "refs/remotes/origin/HEAD"); err == nil {
		return strings.TrimSpace(out), nil
	} else if exitCode(err) < 0 {
		return "", err
	}
	for _, ref := range []string{"refs/heads/main", "refs/heads/master"} {
		if _, err := c.git(ctx, r, "rev-parse", "--verify", "--quiet", ref); err == nil {
			return ref, nil
		} else if exitCode(err) < 0 {
			return "", err
		}
	}
	return "", nil
}

// MergeState is what git knows about whether a branch's work has arrived in the default branch.
type MergeState struct {
	// Base is the default branch's ref, "" when there is none.
	Base string
	// Exists is false when the branch itself is gone.
	Exists bool
	// Ancestor: the branch's tip is contained in Base.
	Ancestor bool
	// AtBase: the branch's tip is Base's tip, so Base has not moved past it.
	AtBase bool
	// UpstreamGone: the branch tracked a remote branch that no longer exists, as after a merged
	// pull request whose branch was deleted and pruned (a squash merge included).
	UpstreamGone bool
	// IsBase: the branch is the default branch itself.
	IsBase bool
}

// Merged is the rule of down --volumes: the branch's commits are in the default branch, or its
// upstream is gone.
func (m MergeState) Merged() bool { return m.Exists && (m.Ancestor || m.UpstreamGone) }

// MergedForGC is the stricter rule of gc: not the default branch, and either the default branch
// has moved past a contained tip or the upstream is gone. A branch without commits of its own,
// which an agent may just have created, does not count.
func (m MergeState) MergedForGC() bool {
	return m.Exists && !m.IsBase && ((m.Ancestor && !m.AtBase) || m.UpstreamGone)
}

// BaseName is the default branch's short name, e.g. main for refs/remotes/origin/main.
func (m MergeState) BaseName() string {
	name := strings.TrimPrefix(m.Base, "refs/heads/")
	if rest, ok := strings.CutPrefix(name, "refs/remotes/"); ok {
		_, name, _ = strings.Cut(rest, "/")
	}
	return name
}

// MergeState returns the merge state of branch.
func (c Client) MergeState(ctx context.Context, r Repo, branch string) (MergeState, error) {
	var m MergeState
	ref := "refs/heads/" + branch
	tip, err := c.git(ctx, r, "rev-parse", "--verify", "--quiet", ref)
	switch {
	case exitCode(err) > 0:
		return m, nil // the branch is gone
	case err != nil:
		return m, err
	}
	m.Exists = true
	track, err := c.git(ctx, r, "for-each-ref", "--format=%(upstream:track)", ref)
	if err != nil {
		return m, err
	}
	m.UpstreamGone = strings.TrimSpace(track) == "[gone]"
	if m.Base, err = c.DefaultBranch(ctx, r); err != nil || m.Base == "" {
		return m, err
	}
	m.IsBase = m.BaseName() == branch
	baseTip, err := c.git(ctx, r, "rev-parse", "--verify", "--quiet", m.Base)
	if err != nil {
		return m, err
	}
	m.AtBase = strings.TrimSpace(tip) == strings.TrimSpace(baseTip)
	_, err = c.git(ctx, r, "merge-base", "--is-ancestor", ref, m.Base)
	switch code := exitCode(err); {
	case err == nil:
		m.Ancestor = true
	case code == 1:
	default:
		return m, err
	}
	return m, nil
}
