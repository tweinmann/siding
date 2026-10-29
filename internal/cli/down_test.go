package cli

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tweinmann/siding/internal/docker"
	"github.com/tweinmann/siding/internal/fail"
	"github.com/tweinmann/siding/internal/stack"
	"github.com/tweinmann/siding/internal/testutil"
)

// downEnv is a stack on branch with a network and a volume.
func downEnv(t *testing.T, branch string) *env {
	t.Helper()
	e := newEnv(t)
	if branch != "main" {
		e.git("switch", "-q", "-c", branch)
		e.git("commit", "-q", "--allow-empty", "-m", "work")
	}
	id := e.id()
	id.Branch = branch
	e.proxyUp(80)
	e.stackUp(id, "db", "web")
	project := map[string]string{stack.LabelProject: id.Project()}
	e.docker.networks = []docker.Network{{Name: id.Project() + "_default", Labels: project}, {Name: "siding", Labels: map[string]string{}}}
	e.docker.volumes = []docker.Volume{{Name: id.Project() + "_data", Labels: project}, {Name: "shared-cache", Labels: map[string]string{}}}
	return e
}

func TestDown(t *testing.T) {
	t.Parallel()

	t.Run("keeps the volumes", func(t *testing.T) {
		t.Parallel()
		e := downEnv(t, "main")
		code, stdout, _ := e.run("down", "--json")
		if code != exitOK {
			t.Fatalf("exit %d: %s", code, stdout)
		}
		testutil.Golden(t, filepath.Join("testdata", "json", "down.json"), []byte(stdout))
		runs := e.compose.recorded()
		if len(runs) != 1 || runs[0].inv.Project != "shop-main" || len(runs[0].inv.Files) != 0 ||
			!slices.Equal(runs[0].args, []string{"down", "--remove-orphans"}) {
			t.Errorf("compose runs %+v", runs)
		}
	})

	t.Run("dry run", func(t *testing.T) {
		t.Parallel()
		e := downEnv(t, "main")
		code, stdout, _ := e.run("down", "--volumes", "--dry-run", "--json")
		if code != exitOK || len(e.compose.recorded()) != 0 {
			t.Fatalf("exit %d, compose runs %d", code, len(e.compose.recorded()))
		}
		testutil.Golden(t, filepath.Join("testdata", "json", "down-dryrun.json"), []byte(stdout))

		_, text, _ := e.run("down", "--dry-run")
		want := "Would remove from shop-main:\n  container shop-main-db-1\n  container shop-main-web-1\n  network shop-main_default\n" +
			"Volumes stay; siding down --volumes removes them.\nDry run: nothing was removed.\n"
		if text != want {
			t.Errorf("text:\n%s\nwant:\n%s", text, want)
		}
	})

	t.Run("no volumes to keep", func(t *testing.T) {
		t.Parallel()
		e := downEnv(t, "main")
		e.docker.volumes = nil
		if _, text, _ := e.run("down"); strings.Contains(text, "Volumes stay") {
			t.Errorf("a stack without volumes was told they stay:\n%s", text)
		}
	})

	t.Run("volumes of a merged, clean branch", func(t *testing.T) {
		t.Parallel()
		e := downEnv(t, "main")
		if code, stdout, _ := e.run("down", "--volumes", "--json"); code != exitOK || !strings.Contains(stdout, "shop-main_data") {
			t.Fatalf("exit %d: %s", code, stdout)
		}
		if runs := e.compose.recorded(); len(runs) != 1 || !slices.Equal(runs[0].args, []string{"down", "--remove-orphans", "--volumes"}) {
			t.Errorf("compose runs %+v", runs)
		}
	})

	for _, tt := range []struct {
		name   string
		branch string
		dirty  bool
		code   string
	}{
		{name: "unmerged branch", branch: "wip", code: fail.UnmergedBranch},
		{name: "dirty worktree", branch: "main", dirty: true, code: fail.DirtyWorktree},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := downEnv(t, tt.branch)
			if tt.dirty {
				e.write("notes.txt", "unsaved")
			}
			code, _, stderr := e.run("down", "--volumes")
			if code != exitFailed || !strings.Contains(stderr, "--force") || len(e.compose.recorded()) != 0 {
				t.Fatalf("exit %d, stderr %q, compose runs %d", code, stderr, len(e.compose.recorded()))
			}
			if _, stdout, _ := e.run("down", "--volumes", "--json"); !strings.Contains(stdout, `"code": "`+tt.code+`"`) {
				t.Errorf("stdout %s, want code %s", stdout, tt.code)
			}
			// Without --volumes nothing is lost, so there is no guard.
			if code, _, _ := e.run("down"); code != exitOK {
				t.Errorf("down without --volumes: exit %d", code)
			}
			if code, _, _ := e.run("down", "--volumes", "--force"); code != exitOK {
				t.Errorf("forced: exit %d", code)
			}
		})
	}

	t.Run("squash-merged: upstream gone", func(t *testing.T) {
		t.Parallel()
		e := downEnv(t, "feat")
		origin := filepath.Join(e.base, "origin.git")
		e.git("init", "-q", "--bare", origin)
		e.git("remote", "add", "origin", origin)
		e.git("push", "-q", "-u", "origin", "feat")
		e.git("push", "-q", "origin", "--delete", "feat") // the PR was merged and its branch deleted
		if code, stdout, _ := e.run("down", "--volumes", "--json"); code != exitOK || !strings.Contains(stdout, "shop-feat_data") {
			t.Errorf("exit %d, stdout %s", code, stdout)
		}
	})

	t.Run("no stack", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t)
		if code, stdout, _ := e.run("down", "--json"); code != exitFailed || !strings.Contains(stdout, `"code": "StackNotFound"`) {
			t.Errorf("exit %d, stdout %s", code, stdout)
		}
	})
}

// After siding down only the volumes are left; down --volumes still finds the stack by them.
func TestDownOnlyData(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	id := e.id()
	labels := id.Labels()
	labels[stack.LabelProject] = id.Project()
	e.docker.volumes = []docker.Volume{{Name: "shop-main_data", Labels: labels}}
	if code, stdout, _ := e.run("down", "--volumes", "--json"); code != exitOK || !strings.Contains(stdout, "shop-main_data") {
		t.Errorf("exit %d, %s", code, stdout)
	}
	if runs := e.compose.recorded(); len(runs) != 1 || !slices.Equal(runs[0].args, []string{"down", "--remove-orphans", "--volumes"}) {
		t.Errorf("compose %+v", runs)
	}
	// status still says there is no stack.
	if code, stdout, _ := e.run("status", "--json"); code != exitFailed || !strings.Contains(stdout, `"code": "StackNotFound"`) {
		t.Errorf("status: exit %d, %s", code, stdout)
	}
}
