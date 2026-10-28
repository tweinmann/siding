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

func TestMerged(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("merged and unmerged against main", func(t *testing.T) {
		t.Parallel()
		dir := newRepo(t)
		sh(t, dir, "branch", "done")
		sh(t, dir, "switch", "-q", "-c", "wip")
		sh(t, dir, "commit", "-q", "--allow-empty", "-m", "wip")
		for branch, want := range map[string]bool{"done": true, "wip": false, "main": true} {
			merged, base, err := Client{}.Merged(ctx, dir, branch)
			if err != nil || merged != want || base != "refs/heads/main" {
				t.Errorf("%s: merged %v, base %q, err %v; want %v against refs/heads/main", branch, merged, base, err, want)
			}
		}
	})

	t.Run("origin/HEAD wins", func(t *testing.T) {
		t.Parallel()
		origin := newRepo(t)
		clone := filepath.Join(t.TempDir(), "clone")
		sh(t, filepath.Dir(clone), "clone", "-q", origin, clone)
		merged, base, err := Client{}.Merged(ctx, clone, "main")
		if err != nil || !merged || base != "refs/remotes/origin/main" {
			t.Errorf("merged %v, base %q, err %v; want true against refs/remotes/origin/main", merged, base, err)
		}
	})

	t.Run("no default branch", func(t *testing.T) {
		t.Parallel()
		dir := filepath.Join(t.TempDir(), "r")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		sh(t, dir, "init", "-q", "-b", "trunk")
		sh(t, dir, "commit", "-q", "--allow-empty", "-m", "first")
		merged, base, err := Client{}.Merged(ctx, dir, "trunk")
		if err != nil || merged || base != "" {
			t.Errorf("merged %v, base %q, err %v; want unmerged without a base", merged, base, err)
		}
	})
}
