package cli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tweinmann/siding/internal/fail"
	"github.com/tweinmann/siding/internal/testutil"
)

func TestSetup(t *testing.T) {
	t.Parallel()

	t.Run("first setup", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t)
		code, stdout, stderr := e.run("setup", "--json")
		if code != exitOK || stderr != "" {
			t.Fatalf("exit %d, stderr %q", code, stderr)
		}
		testutil.Golden(t, filepath.Join("testdata", "json", "setup.json"), []byte(stdout))
		if !slices.Equal(e.docker.ensured, []string{"siding"}) {
			t.Errorf("networks ensured: %v", e.docker.ensured)
		}
		runs := e.compose.recorded()
		if len(runs) != 1 || runs[0].inv.Project != "siding" || runs[0].args[0] != "up" {
			t.Fatalf("compose runs %+v", runs)
		}
		data, err := os.ReadFile(runs[0].inv.Files[0])
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{`"127.0.0.1:80:80"`, `container_name: "siding-proxy"`, "LabelRegex(`dev.siding.repo`, `.+`)",
			"Label(`traefik.docker.network`, `siding`)", "--providers.docker.exposedbydefault=false"} {
			if !strings.Contains(string(data), want) {
				t.Errorf("the proxy's compose file lacks %s:\n%s", want, data)
			}
		}
		if !strings.HasPrefix(runs[0].inv.Files[0], e.opts.Home+"/.siding/proxy/") {
			t.Errorf("compose file %s is not under ~/.siding", runs[0].inv.Files[0])
		}
	})

	t.Run("already running", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t)
		e.proxyUp(80)
		code, stdout, _ := e.run("setup")
		if code != exitOK || stdout != "siding-proxy already runs on http://*.localhost/\n" || len(e.compose.recorded()) != 0 {
			t.Errorf("exit %d, stdout %q, compose runs %d", code, stdout, len(e.compose.recorded()))
		}
	})

	t.Run("keeps the running port without --port", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t)
		e.proxyUp(8080)
		if code, stdout, _ := e.run("setup"); code != exitOK || !strings.Contains(stdout, "already runs on http://*.localhost:8080/") {
			t.Errorf("exit %d, stdout %q", code, stdout)
		}
	})

	t.Run("another port recreates the proxy", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t)
		e.proxyUp(80)
		code, stdout, _ := e.run("setup", "--port", "8080")
		if code != exitOK || stdout != "siding-proxy restarted on http://*.localhost:8080/\n" || len(e.compose.recorded()) != 1 {
			t.Errorf("exit %d, stdout %q", code, stdout)
		}
	})

	t.Run("port in use", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t)
		e.compose.runErr = fail.New(fail.ComposeFailed, "docker compose up failed: Error response from daemon: failed to bind host port 127.0.0.1:80/tcp: address already in use")
		code, _, stderr := e.run("setup")
		if code != exitFailed || !strings.Contains(stderr, "port 80 on 127.0.0.1 is in use") || !strings.Contains(stderr, "--port") {
			t.Errorf("exit %d, stderr %q", code, stderr)
		}
	})
}
