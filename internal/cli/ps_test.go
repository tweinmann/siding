package cli

import (
	"path/filepath"
	"testing"

	"github.com/tweinmann/siding/internal/docker"
	"github.com/tweinmann/siding/internal/queue"
	"github.com/tweinmann/siding/internal/testutil"
)

func TestPs(t *testing.T) {
	t.Parallel()

	t.Run("stacks", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t)
		e.proxyUp(8080)
		main := e.id()
		login := e.id()
		login.Branch, login.Worktree, login.Agent = "feature/login", filepath.Join(e.base, "shop-login"), "claude"
		e.stackUp(main, "web", "db")
		e.stackUp(login, "web", "db")
		e.docker.update("db", func(c *docker.Container) {
			if c.Labels["dev.siding.branch"] == "feature/login" {
				c.State = "exited"
			}
		})
		e.docker.add(docker.Container{ID: "foreign", Labels: map[string]string{"com.docker.compose.project": "other"}, State: "running"})
		const mib = 1 << 20
		e.docker.memory["shop-main-web-1"] = 200 * mib
		e.docker.memory["shop-main-db-1"] = 100 * mib
		e.docker.memory["shop-feature-login-web-1"] = 500 * mib

		// An up waits in line for memory.
		if _, err := queue.Open(e.opts.Home, e.opts.Alive).Enter(queue.Entry{Project: "shop-feature-x", PID: 777, Estimate: 768 * mib, Since: e.clock.now}); err != nil {
			t.Fatal(err)
		}

		code, stdout, _ := e.run("ps", "--json")
		if code != exitOK {
			t.Fatalf("exit %d", code)
		}
		testutil.Golden(t, filepath.Join("testdata", "json", "stacklist.json"), []byte(stdout))

		_, text, _ := e.run("ps")
		want := `REPO  BRANCH         STATE    URL                                        MEMORY   WORKTREE
shop  feature/login  partial  http://shop.feature-login.localhost:8080/  500 MiB  /src/shop-login (missing)
shop  main           running  http://shop.main.localhost:8080/           300 MiB  /src/shop
total: 800 MiB of 12.0 GiB
waiting for memory:
  1.  shop-feature-x  needs 768 MiB  since 2026-10-28T12:00:00Z
`
		if text != want {
			t.Errorf("text:\n%s\nwant:\n%s", text, want)
		}
	})

	t.Run("no stacks", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t)
		code, stdout, _ := e.run("ps", "--json")
		if code != exitOK {
			t.Fatalf("exit %d", code)
		}
		testutil.Golden(t, filepath.Join("testdata", "json", "stacklist-empty.json"), []byte(stdout))
	})

	t.Run("from outside any repository", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t)
		e.stackUp(e.id(), "web")
		e.opts.Dir = t.TempDir()
		if code, stdout, _ := e.run("ps"); code != exitOK || stdout == "" {
			t.Errorf("exit %d, stdout %q", code, stdout)
		}
	})
}

func TestBytesText(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		n    uint64
		want string
	}{{0, "0 MiB"}, {16, "<1 MiB"}, {1 << 19, "1 MiB"}, {7 << 20, "7 MiB"}, {3 << 29, "1.5 GiB"}} {
		if got := bytesText(tt.n); got != tt.want {
			t.Errorf("bytesText(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}
