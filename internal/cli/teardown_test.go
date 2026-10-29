package cli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tweinmann/siding/internal/testutil"
)

func TestTeardown(t *testing.T) {
	t.Parallel()

	t.Run("nothing runs but the proxy", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t)
		setupDoc(t, e)
		machineID := filepath.Join(e.opts.Home, ".siding", "machine-id")
		if err := os.WriteFile(machineID, []byte("0123456789abcdef\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		code, stdout, _ := e.run("teardown", "--json")
		if code != exitOK {
			t.Fatalf("exit %d: %s", code, stdout)
		}
		testutil.Golden(t, filepath.Join("testdata", "json", "teardown.json"), []byte(stdout))
		if e.docker.proxy != nil || !slices.Equal(e.docker.removedNetworks, []string{"siding"}) {
			t.Errorf("proxy %v, removed networks %v", e.docker.proxy, e.docker.removedNetworks)
		}
		if _, err := os.Stat(filepath.Join(e.opts.Home, ".siding", "proxy")); !os.IsNotExist(err) {
			t.Errorf("~/.siding/proxy is still there: %v", err)
		}
		if _, err := os.Stat(machineID); err != nil {
			t.Errorf("the machine id is gone: %v", err)
		}
	})

	t.Run("a stack still runs", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t)
		setupDoc(t, e)
		e.stackUp(e.id(), "web")
		code, stdout, _ := e.run("teardown")
		want := "Removed container siding-proxy\nKept network siding: stacks still use it\n" +
			"These stacks keep running, without the proxy: shop-main\nRemove them with siding down in their worktree, or siding gc.\n"
		if code != exitOK || stdout != want {
			t.Errorf("exit %d, stdout:\n%s\nwant:\n%s", code, stdout, want)
		}
		if len(e.docker.removedNetworks) != 0 {
			t.Errorf("removed networks %v", e.docker.removedNetworks)
		}
		for _, r := range e.compose.recorded() {
			if r.inv.Project != "siding" {
				t.Errorf("teardown touched the stack %s", r.inv.Project)
			}
		}
	})

	t.Run("dry run", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t)
		setupDoc(t, e)
		runs := len(e.compose.recorded())
		code, stdout, _ := e.run("teardown", "--dry-run", "--json")
		if code != exitOK || len(e.compose.recorded()) != runs || e.docker.proxy == nil || len(e.docker.removedNetworks) != 0 {
			t.Fatalf("exit %d, the dry run removed something", code)
		}
		testutil.Golden(t, filepath.Join("testdata", "json", "teardown-dryrun.json"), []byte(stdout))
	})

	t.Run("no proxy", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t)
		if code, stdout, _ := e.run("teardown"); code != exitOK || !strings.HasPrefix(stdout, "Nothing to remove") {
			t.Errorf("exit %d, stdout %q", code, stdout)
		}
	})
}

func TestTeardownSnapshots(t *testing.T) {
	t.Parallel()
	e := snapshotEnv(t)
	setupDoc(t, e)
	code, stdout, _ := e.run("teardown", "--dry-run")
	if code != exitOK || !strings.Contains(stdout, "Kept the snapshots blog/base, shop/base, shop/big; siding teardown --snapshots removes them.") {
		t.Errorf("without --snapshots: exit %d\n%s", code, stdout)
	}
	if code, _, _ := e.run("teardown", "--snapshots"); code != exitOK {
		t.Fatal("teardown --snapshots failed")
	}
	var removed []string
	for _, c := range e.docker.calls {
		removed = append(removed, strings.TrimPrefix(c, "remove "))
	}
	slices.Sort(removed)
	want := []string{"siding-snap_blog_base_data", "siding-snap_shop_base_data", "siding-snap_shop_base_uploads", "siding-snap_shop_big_data"}
	if !slices.Equal(removed, want) {
		t.Errorf("removed %q, want %q (the other machine's snapshot stays)", removed, want)
	}
}
