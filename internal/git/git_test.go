package git

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/tweinmann/siding/internal/fail"
)

// sh runs git in dir with a fixed identity and without the developer's configuration.
func sh(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// newRepo creates <tmp>/shop with one commit on main and returns its path.
func newRepo(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "shop")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	sh(t, dir, "init", "-q", "-b", "main")
	sh(t, dir, "commit", "-q", "--allow-empty", "-m", "first")
	return dir
}

func code(err error) string {
	var e *fail.Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

func TestWorktree(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	main := newRepo(t)
	linked := filepath.Join(filepath.Dir(main), "shop-login")
	sh(t, main, "worktree", "add", "-q", "-b", "feature/login", linked)
	sub := filepath.Join(linked, "web", "src")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	detached := filepath.Join(filepath.Dir(main), "shop-detached")
	sh(t, main, "worktree", "add", "-q", "--detach", detached)

	tests := []struct {
		name, dir, top, branch, code string
	}{
		{name: "main worktree", dir: main, top: main, branch: "main"},
		{name: "linked worktree from a subdirectory", dir: sub, top: linked, branch: "feature/login"},
		{name: "detached", dir: detached, code: fail.NoBranch},
		{name: "outside", dir: t.TempDir(), code: fail.NotAGitRepository},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w, err := Client{}.Worktree(ctx, tt.dir)
			if tt.code != "" {
				if code(err) != tt.code {
					t.Fatalf("err %v, want code %s", err, tt.code)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if w.Top != tt.top || w.Branch != tt.branch || w.RepoName() != "shop" {
				t.Errorf("got %+v (repo %s), want top %s, branch %s, repo shop", w, w.RepoName(), tt.top, tt.branch)
			}
		})
	}
}

func TestDirty(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := newRepo(t)
	dirty, err := Client{}.Dirty(ctx, dir)
	if err != nil || dirty {
		t.Fatalf("fresh repository: dirty %v, err %v", dirty, err)
	}
	// .siding/ ignores itself, so siding's own files never make a worktree dirty.
	if err := os.MkdirAll(filepath.Join(dir, ".siding"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{".siding/.gitignore": "*\n", ".siding/override.yml": "services: {}\n"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if dirty, err := (Client{}).Dirty(ctx, dir); err != nil || dirty {
		t.Fatalf("with .siding/: dirty %v, err %v", dirty, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if dirty, err := (Client{}).Dirty(ctx, dir); err != nil || !dirty {
		t.Fatalf("with an untracked file: dirty %v, err %v", dirty, err)
	}
}

func TestMergeState(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// shop: main, "fresh" at main's tip, "done" merged with main one commit further, "wip"
	// with a commit of its own.
	dir := newRepo(t)
	sh(t, dir, "branch", "done")
	sh(t, dir, "commit", "-q", "--allow-empty", "-m", "after done")
	sh(t, dir, "branch", "fresh")
	sh(t, dir, "switch", "-q", "-c", "wip")
	sh(t, dir, "commit", "-q", "--allow-empty", "-m", "wip")
	sh(t, dir, "switch", "-q", "main")

	// clone: "feat" tracked a remote branch that was squash-merged and deleted, then pruned.
	origin := newRepo(t)
	sh(t, origin, "switch", "-q", "-c", "feat")
	sh(t, origin, "commit", "-q", "--allow-empty", "-m", "feat")
	sh(t, origin, "switch", "-q", "main")
	clone := filepath.Join(t.TempDir(), "clone")
	sh(t, filepath.Dir(clone), "clone", "-q", origin, clone)
	sh(t, clone, "switch", "-q", "feat")
	sh(t, clone, "switch", "-q", "main")
	sh(t, origin, "branch", "-q", "-D", "feat")
	sh(t, clone, "fetch", "-q", "--prune")

	// gone: the worktree of a merged branch was deleted without git; only its git dir is left.
	gone := newRepo(t)
	wt := filepath.Join(t.TempDir(), "gone-wt")
	sh(t, gone, "worktree", "add", "-q", "-b", "old", wt)
	sh(t, gone, "commit", "-q", "--allow-empty", "-m", "main moved on")
	if err := os.RemoveAll(wt); err != nil {
		t.Fatal(err)
	}

	noBase := filepath.Join(t.TempDir(), "r")
	if err := os.MkdirAll(noBase, 0o755); err != nil {
		t.Fatal(err)
	}
	sh(t, noBase, "init", "-q", "-b", "trunk")
	sh(t, noBase, "commit", "-q", "--allow-empty", "-m", "first")

	tests := []struct {
		name          string
		repo          Repo
		branch        string
		merged, forGC bool
		base          string
	}{
		{name: "a fresh branch is merged but not for gc", repo: Repo{Dir: dir}, branch: "fresh", merged: true, base: "refs/heads/main"},
		{name: "merged and main moved on", repo: Repo{Dir: dir}, branch: "done", merged: true, forGC: true, base: "refs/heads/main"},
		{name: "own commits", repo: Repo{Dir: dir}, branch: "wip", base: "refs/heads/main"},
		{name: "the default branch", repo: Repo{Dir: dir}, branch: "main", merged: true, base: "refs/heads/main"},
		{name: "upstream gone after a squash merge", repo: Repo{Dir: clone}, branch: "feat", merged: true, forGC: true, base: "refs/remotes/origin/main"},
		{name: "the worktree is gone", repo: Repo{GitDir: filepath.Join(gone, ".git")}, branch: "old", merged: true, forGC: true, base: "refs/heads/main"},
		{name: "the branch is gone", repo: Repo{Dir: dir}, branch: "nope"},
		{name: "no default branch", repo: Repo{Dir: noBase}, branch: "trunk"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m, err := Client{}.MergeState(ctx, tt.repo, tt.branch)
			if err != nil {
				t.Fatal(err)
			}
			if m.Merged() != tt.merged || m.MergedForGC() != tt.forGC || m.Base != tt.base {
				t.Errorf("%+v: merged %v, for gc %v; want %v, %v, base %q", m, m.Merged(), m.MergedForGC(), tt.merged, tt.forGC, tt.base)
			}
		})
	}
}

func TestMainWorktree(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	main := newRepo(t)
	linked := filepath.Join(filepath.Dir(main), "shop-feature-x")
	sh(t, main, "worktree", "add", "-q", "-b", "feature/x", linked)
	c := Client{}
	for _, tt := range []struct {
		dir    string
		linked bool
	}{{main, false}, {linked, true}} {
		w, err := c.Worktree(ctx, tt.dir)
		if err != nil {
			t.Fatal(err)
		}
		real, _ := filepath.EvalSymlinks(main)
		if w.MainTop() != real || w.Linked() != tt.linked {
			t.Errorf("%s: main top %q, linked %v", tt.dir, w.MainTop(), w.Linked())
		}
	}
	// A common git directory not named .git has no main worktree.
	if w := (Worktree{Top: "/src/shop", CommonDir: "/src/shop.git"}); w.MainTop() != "" || w.Linked() {
		t.Errorf("bare: %q %v", w.MainTop(), w.Linked())
	}
}

func TestTracked(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := newRepo(t)
	for name, content := range map[string]string{".env.example": "A=1\n", ".env": "A=not-a-real-token\n"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	sh(t, dir, "add", ".env.example")
	c := Client{}
	for path, want := range map[string]bool{".env.example": true, ".env": false, "missing.env": false} {
		got, err := c.Tracked(ctx, dir, path)
		if err != nil || got != want {
			t.Errorf("%s: %v, %v, want %v", path, got, err, want)
		}
	}
}

func TestTop(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := newRepo(t)
	sh(t, dir, "checkout", "-q", "--detach")
	sub := filepath.Join(dir, "web")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	real, _ := filepath.EvalSymlinks(dir)
	if top, err := (Client{}).Top(ctx, sub); err != nil || top != real {
		t.Errorf("detached, in a subdirectory: %q, %v", top, err)
	}
	if _, err := (Client{}).Top(ctx, t.TempDir()); code(err) != fail.NotAGitRepository {
		t.Errorf("outside: %v", err)
	}
}
