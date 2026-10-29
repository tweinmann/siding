package cli

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tweinmann/siding/internal/docker"
	"github.com/tweinmann/siding/internal/fail"
	"github.com/tweinmann/siding/internal/snapshot"
	"github.com/tweinmann/siding/internal/testutil"
)

// composeCalls are the compose runs as "<project> <args>".
func composeCalls(e *env) []string {
	var out []string
	for _, r := range e.compose.recorded() {
		out = append(out, r.inv.Project+" "+strings.Join(r.args, " "))
	}
	return out
}

func TestSnapshotSave(t *testing.T) {
	t.Parallel()

	t.Run("save", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t)
		e.stackUp(e.id(), "db", "web")
		code, stdout, stderr := e.run("snapshot", "save", "base", "--json")
		if code != exitOK {
			t.Fatalf("exit %d: %s %s", code, stdout, stderr)
		}
		testutil.Golden(t, filepath.Join("testdata", "json", "snapshot.json"), []byte(stdout))
		if got, want := composeCalls(e), []string{"shop-main stop db", "shop-main start db"}; !slices.Equal(got, want) {
			t.Errorf("compose %q, want %q", got, want)
		}
		if got, want := e.docker.calls, []string{"create siding-snap_shop_base_data", "copy shop-main_data siding-snap_shop_base_data"}; !slices.Equal(got, want) {
			t.Errorf("docker %q, want %q", got, want)
		}
		if !strings.Contains(stderr, "Stopping db while copying") {
			t.Errorf("stderr %q", stderr)
		}
	})

	t.Run("a taken name", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t)
		e.stackUp(e.id(), "db")
		if code, _, _ := e.run("snapshot", "save", "base"); code != exitOK {
			t.Fatal("first save failed")
		}
		if code, stdout, _ := e.run("snapshot", "save", "base", "--json"); code != exitFailed || !strings.Contains(stdout, `"code": "SnapshotExists"`) {
			t.Errorf("second save: exit %d, %s", code, stdout)
		}
		e.docker.calls = nil
		if code, _, _ := e.run("snapshot", "save", "base", "--force"); code != exitOK {
			t.Fatal("--force failed")
		}
		if got, want := e.docker.calls, []string{"remove siding-snap_shop_base_data", "create siding-snap_shop_base_data", "copy shop-main_data siding-snap_shop_base_data"}; !slices.Equal(got, want) {
			t.Errorf("--force: %q, want %q", got, want)
		}
	})

	t.Run("the copy fails", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t)
		e.stackUp(e.id(), "db")
		e.docker.helperExit = 1
		code, _, stderr := e.run("snapshot", "save", "base")
		if code != exitFailed || !strings.Contains(stderr, "No space left on device") {
			t.Errorf("exit %d, stderr %q", code, stderr)
		}
		if got := composeCalls(e); !slices.Equal(got, []string{"shop-main stop db", "shop-main start db"}) {
			t.Errorf("the services were not started again: %q", got)
		}
		if slices.ContainsFunc(e.docker.volumes, func(v docker.Volume) bool { return strings.HasPrefix(v.Name, "siding-snap_") }) {
			t.Error("a half snapshot is left")
		}
	})

	for _, tt := range []struct {
		name, config string
		args         []string
	}{
		{name: "no volumes", config: `{"services": {"web": {"expose": ["80"]}}}`, args: []string{"snapshot", "save", "base", "--json"}},
		{name: "a bad name", args: []string{"snapshot", "save", "Base_1", "--json"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := newEnv(t)
			if tt.config != "" {
				e.compose.config = tt.config
			}
			e.stackUp(e.id(), "web")
			code, stdout, _ := e.run(tt.args...)
			if code != exitFailed || !strings.Contains(stdout, `"code": "`+fail.InvalidArguments+`"`) || len(e.docker.calls) != 0 {
				t.Errorf("exit %d, %s, docker %q", code, stdout, e.docker.calls)
			}
		})
	}
}

// snapshotEnv has the snapshots shop/base, shop/big and blog/base of this machine and one of
// another machine.
func snapshotEnv(t *testing.T) *env {
	t.Helper()
	e := newEnv(t)
	machine := e.id().Machine
	add := func(repo, name, key, m string) {
		e.docker.volumes = append(e.docker.volumes, docker.Volume{Name: snapshot.VolumeName(repo, name, key),
			Labels: snapshot.Labels(repo, name, key, m, e.clock.now)})
	}
	add("shop", "base", "data", machine)
	add("shop", "base", "uploads", machine)
	add("shop", "big", "data", machine)
	add("blog", "base", "data", machine)
	add("shop", "remote", "data", "another-machine")
	return e
}

func TestSnapshotLsRm(t *testing.T) {
	t.Parallel()

	t.Run("ls in a repository", func(t *testing.T) {
		t.Parallel()
		e := snapshotEnv(t)
		code, stdout, _ := e.run("snapshot", "ls", "--json")
		if code != exitOK {
			t.Fatalf("exit %d", code)
		}
		testutil.Golden(t, filepath.Join("testdata", "json", "snapshotlist.json"), []byte(stdout))
		_, text, _ := e.run("snapshot", "ls")
		want := "REPO  NAME  VOLUMES        SIZE   CREATED\nshop  base  data, uploads  2 MiB  2026-10-28T12:00:00Z\nshop  big   data           1 MiB  2026-10-28T12:00:00Z\n"
		if text != want {
			t.Errorf("text:\n%s\nwant:\n%s", text, want)
		}
	})

	t.Run("ls outside a repository", func(t *testing.T) {
		t.Parallel()
		e := snapshotEnv(t)
		e.opts.Dir = t.TempDir()
		_, stdout, _ := e.run("snapshot", "ls")
		if !strings.Contains(stdout, "blog  base") || strings.Contains(stdout, "remote") {
			t.Errorf("stdout:\n%s", stdout)
		}
	})

	t.Run("rm", func(t *testing.T) {
		t.Parallel()
		e := snapshotEnv(t)
		if code, stdout, _ := e.run("snapshot", "rm", "base", "--dry-run"); code != exitOK || !strings.HasPrefix(stdout, "Would remove the snapshot base of shop: data, uploads") || len(e.docker.calls) != 0 {
			t.Errorf("dry run: exit %d, %q, %q", code, stdout, e.docker.calls)
		}
		if code, _, _ := e.run("snapshot", "rm", "base"); code != exitOK {
			t.Fatal("rm failed")
		}
		want := []string{"remove siding-snap_shop_base_data", "remove siding-snap_shop_base_uploads"}
		if !slices.Equal(e.docker.calls, want) {
			t.Errorf("removed %q, want %q", e.docker.calls, want)
		}
		if code, stdout, _ := e.run("snapshot", "rm", "nope", "--json"); code != exitFailed || !strings.Contains(stdout, `"code": "SnapshotNotFound"`) || !strings.Contains(stdout, "big") {
			t.Errorf("unknown: exit %d, %s", code, stdout)
		}
	})
}
