package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tweinmann/siding/internal/fail"
	"github.com/tweinmann/siding/internal/testutil"
)

func TestUp(t *testing.T) {
	t.Parallel()

	t.Run("starts the stack", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t)
		e.proxyUp(80)
		before, _ := os.ReadFile(filepath.Join(e.repo, "compose.yaml"))
		code, stdout, stderr := e.run("up", "--json", "--", "--build", "web")
		if code != exitOK || stderr != "" {
			t.Fatalf("exit %d, stderr %q", code, stderr)
		}
		testutil.Golden(t, filepath.Join("testdata", "json", "stack.json"), []byte(stdout))

		runs := e.compose.recorded()
		if len(runs) != 1 {
			t.Fatalf("compose runs %+v", runs)
		}
		r := runs[0]
		wantFiles := []string{filepath.Join(e.repo, "compose.yaml"), filepath.Join(e.repo, ".siding", "override.yml")}
		if r.inv.Project != "shop-main" || !slices.Equal(r.inv.Files, wantFiles) ||
			!slices.Equal(r.args, []string{"up", "-d", "--remove-orphans", "--build", "web"}) {
			t.Errorf("compose ran %+v", r)
		}
		override, err := os.ReadFile(wantFiles[1])
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(override), "ports: !reset []") || !strings.Contains(string(override), "Host(`shop.main.localhost`) && PathPrefix(`/api`)") {
			t.Errorf("override:\n%s", override)
		}
		after, _ := os.ReadFile(filepath.Join(e.repo, "compose.yaml"))
		if string(after) != string(before) {
			t.Error("the compose file changed")
		}
		if out, err := exec.Command("git", "-C", e.repo, "status", "--porcelain").Output(); err != nil || len(out) != 0 {
			t.Errorf("the worktree is dirty after up: %q, %v", out, err)
		}
	})

	t.Run("text", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t)
		e.proxyUp(8080)
		code, stdout, _ := e.run("up")
		want := `shop-main is up at http://shop.main.localhost:8080/
  api   http://api.shop.main.localhost:8080/   route /api
  mail  http://mail.shop.main.localhost:8080/
  web   http://web.shop.main.localhost:8080/   route /
Wait for the verdict with: siding status --wait
`
		if code != exitOK || stdout != want {
			t.Errorf("exit %d, stdout:\n%s\nwant:\n%s", code, stdout, want)
		}
	})

	t.Run("agent", func(t *testing.T) {
		t.Parallel()
		for _, tt := range []struct {
			name, env, flag, want string
		}{
			{name: "from SIDING_AGENT", env: "claude", want: `"dev.siding.agent": "claude"`},
			{name: "the flag wins", env: "claude", flag: "codex", want: `"dev.siding.agent": "codex"`},
			{name: "none", want: ""},
		} {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				e := newEnv(t)
				e.proxyUp(80)
				e.envVars["SIDING_AGENT"] = tt.env
				args := []string{"up"}
				if tt.flag != "" {
					args = append(args, "--agent", tt.flag)
				}
				if code, _, stderr := e.run(args...); code != exitOK {
					t.Fatalf("exit %d: %s", code, stderr)
				}
				override, _ := os.ReadFile(filepath.Join(e.repo, ".siding", "override.yml"))
				got := strings.Contains(string(override), "dev.siding.agent")
				if (tt.want == "") == got || (tt.want != "" && !strings.Contains(string(override), tt.want)) {
					t.Errorf("override has agent %v, want %q", got, tt.want)
				}
			})
		}
	})

	t.Run("repo override", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t)
		e.proxyUp(80)
		if code, stdout, _ := e.run("up", "--repo", "store"); code != exitOK || !strings.HasPrefix(stdout, "store-main is up at http://store.main.localhost/") {
			t.Errorf("exit %d, stdout %q", code, stdout)
		}
	})

	failures := []struct {
		name  string
		setup func(e *env)
		code  string
	}{
		{name: "proxy not running", setup: func(*env) {}, code: fail.ProxyNotRunning},
		{name: "outside a repository", setup: func(e *env) { e.proxyUp(80); e.opts.Dir = t.TempDir() }, code: fail.NotAGitRepository},
		{name: "detached HEAD", setup: func(e *env) { e.proxyUp(80); e.git("checkout", "-q", "--detach") }, code: fail.NoBranch},
		{name: "no compose file", setup: func(e *env) {
			e.proxyUp(80)
			_ = os.Remove(filepath.Join(e.repo, "compose.yaml"))
		}, code: fail.ComposeFileNotFound},
		{name: "slug collision", setup: func(e *env) {
			e.proxyUp(80)
			other := e.id()
			other.Branch, other.Worktree = "main", "/src/elsewhere"
			e.docker.add(container(other, "web", "running", "", 0))
		}, code: fail.SlugCollision},
		{name: "compose fails", setup: func(e *env) {
			e.proxyUp(80)
			e.compose.runErr = fail.New(fail.ComposeFailed, "docker compose up -d failed: pull access denied for nope")
		}, code: fail.ComposeFailed},
		{name: "two services on one path", setup: func(e *env) {
			e.proxyUp(80)
			e.compose.config = `{"services": {"a": {"expose": ["80"], "labels": {"dev.siding.route": "/"}}, "b": {"expose": ["80"], "labels": {"dev.siding.route": "/"}}}}`
		}, code: fail.InvalidRoute},
	}
	for _, tt := range failures {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := newEnv(t)
			tt.setup(e)
			code, stdout, _ := e.run("up", "--json")
			if code != exitFailed || !strings.Contains(stdout, `"code": "`+tt.code+`"`) {
				t.Errorf("exit %d, stdout %s; want code %s", code, stdout, tt.code)
			}
			if tt.code != fail.ComposeFailed && len(e.compose.recorded()) != 0 {
				t.Errorf("compose ran although up failed: %+v", e.compose.recorded())
			}
		})
	}

	t.Run("a plain Compose project of the same name is adopted", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t)
		e.proxyUp(80)
		plain := container(e.id(), "web", "running", "", 0)
		plain.Labels = map[string]string{"com.docker.compose.project": "shop-main", "com.docker.compose.service": "web"}
		e.docker.add(plain)
		if code, _, stderr := e.run("up"); code != exitOK {
			t.Errorf("exit %d: %s", code, stderr)
		}
	})
}
