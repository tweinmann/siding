package cli

import (
	"path/filepath"
	"testing"

	"github.com/tweinmann/siding/internal/docker"
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

		code, stdout, _ := e.run("ps", "--json")
		if code != exitOK {
			t.Fatalf("exit %d", code)
		}
		testutil.Golden(t, filepath.Join("testdata", "json", "stacklist.json"), []byte(stdout))

		_, text, _ := e.run("ps")
		want := `REPO   BRANCH         STATE    URL                                        MEMORY   WORKTREE
shop   feature/login  partial  http://shop.feature-login.localhost:8080/  500 MiB  /src/shop-login (missing)
shop   main           running  http://shop.main.localhost:8080/           300 MiB  /src/shop
total                                                                     800 MiB
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
