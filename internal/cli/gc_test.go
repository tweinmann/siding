package cli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tweinmann/siding/internal/docker"
	"github.com/tweinmann/siding/internal/stack"
	"github.com/tweinmann/siding/internal/testutil"
)

// gcEnv has five stacks: main (the default branch), done (merged, main moved on, clean), wip
// (dirty), old (worktree deleted, branch merged) and one of another machine.
func gcEnv(t *testing.T) *env {
	t.Helper()
	e := newEnv(t)
	e.git("branch", "done")
	e.git("branch", "old")
	e.git("commit", "-q", "--allow-empty", "-m", "main moved on")
	e.git("worktree", "add", "-q", filepath.Join(e.base, "shop-done"), "done")
	e.git("worktree", "add", "-q", "-b", "wip", filepath.Join(e.base, "shop-wip"))
	if err := os.WriteFile(filepath.Join(e.base, "shop-wip", "notes.txt"), []byte("unsaved"), 0o644); err != nil {
		t.Fatal(err)
	}

	add := func(branch, worktree, machine string) stack.Identity {
		id := e.id()
		id.Branch, id.Worktree, id.GitDir = branch, worktree, filepath.Join(e.repo, ".git")
		if machine != "" {
			id.Machine = machine
		}
		e.docker.add(container(id, "web", "running", "", 0))
		project := map[string]string{stack.LabelProject: id.Project()}
		e.docker.networks = append(e.docker.networks, docker.Network{Name: id.Project() + "_default", Labels: project})
		e.docker.volumes = append(e.docker.volumes, docker.Volume{Name: id.Project() + "_data", Labels: project})
		return id
	}
	add("main", e.repo, "")
	add("done", filepath.Join(e.base, "shop-done"), "")
	add("wip", filepath.Join(e.base, "shop-wip"), "")
	add("old", filepath.Join(e.base, "shop-old"), "") // never existed: its worktree is gone
	add("remote", filepath.Join(e.base, "elsewhere"), "another-machine")
	return e
}

func TestGC(t *testing.T) {
	t.Parallel()

	t.Run("dry run", func(t *testing.T) {
		t.Parallel()
		e := gcEnv(t)
		code, stdout, _ := e.run("gc", "--dry-run", "--json")
		if code != exitOK || len(e.compose.recorded()) != 0 {
			t.Fatalf("exit %d, compose runs %d: %s", code, len(e.compose.recorded()), stdout)
		}
		testutil.Golden(t, filepath.Join("testdata", "json", "gc-dryrun.json"), []byte(stdout))
	})

	t.Run("yes", func(t *testing.T) {
		t.Parallel()
		e := gcEnv(t)
		code, stdout, _ := e.run("gc", "--yes", "--json")
		if code != exitOK {
			t.Fatalf("exit %d: %s", code, stdout)
		}
		testutil.Golden(t, filepath.Join("testdata", "json", "gc.json"), []byte(stdout))
		var got []string
		for _, r := range e.compose.recorded() {
			got = append(got, r.inv.Project+" "+strings.Join(r.args, " "))
		}
		want := []string{"shop-done down --remove-orphans --volumes", "shop-old down --remove-orphans --volumes"}
		if !slices.Equal(got, want) {
			t.Errorf("compose ran %q, want %q", got, want)
		}
		for _, dir := range []string{"shop-done", "shop-wip"} {
			if _, err := os.Stat(filepath.Join(e.base, dir)); err != nil {
				t.Errorf("gc touched the worktree %s: %v", dir, err)
			}
		}
	})

	t.Run("asks on a terminal", func(t *testing.T) {
		t.Parallel()
		for _, tt := range []struct {
			answer string
			runs   int
		}{{"y\n", 2}, {"n\n", 0}, {"\n", 0}} {
			e := gcEnv(t)
			e.opts.IsTerminal = func() bool { return true }
			e.opts.Stdin = strings.NewReader(tt.answer)
			code, stdout, stderr := e.run("gc")
			if code != exitOK || len(e.compose.recorded()) != tt.runs || !strings.Contains(stderr, "Remove these? [y/N]") ||
				!strings.Contains(stdout, "Would remove shop-done (done, Merged)") {
				t.Errorf("answer %q: exit %d, compose runs %d, stdout %q, stderr %q", tt.answer, code, len(e.compose.recorded()), stdout, stderr)
			}
		}
	})

	t.Run("nobody to ask", func(t *testing.T) {
		t.Parallel()
		e := gcEnv(t)
		if code, _, stderr := e.run("gc"); code != exitFailed || !strings.Contains(stderr, "--yes") {
			t.Errorf("no terminal: exit %d, stderr %q", code, stderr)
		}
		e.opts.IsTerminal = func() bool { return true }
		if code, stdout, _ := e.run("gc", "--json"); code != exitFailed || !strings.Contains(stdout, `"code": "NotInteractive"`) {
			t.Errorf("--json: exit %d, stdout %s", code, stdout)
		}
		if len(e.compose.recorded()) != 0 {
			t.Error("gc removed something without being asked")
		}
	})

	t.Run("nothing to remove", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t)
		e.stackUp(e.id(), "web")
		if code, stdout, _ := e.run("gc"); code != exitOK || !strings.HasPrefix(stdout, "Nothing to remove.") {
			t.Errorf("exit %d, stdout %q", code, stdout)
		}
	})
}
