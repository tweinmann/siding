package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tweinmann/siding/internal/fail"
	"github.com/tweinmann/siding/internal/testutil"
)

func setupDoc(t *testing.T, e *env, args ...string) setupDocument {
	t.Helper()
	code, stdout, stderr := e.run(append([]string{"setup", "--json"}, args...)...)
	if code != exitOK {
		t.Fatalf("setup %v: exit %d, %s %s", args, code, stdout, stderr)
	}
	var doc setupDocument
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

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
		if strings.Contains(string(data), "dev.siding.budget") {
			t.Error("a default budget must not be a label")
		}
		if !strings.HasPrefix(runs[0].inv.Files[0], e.opts.Home+"/.siding/proxy/") {
			t.Errorf("compose file %s is not under ~/.siding", runs[0].inv.Files[0])
		}
	})

	t.Run("second setup changes nothing", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t)
		setupDoc(t, e)
		code, stdout, _ := e.run("setup")
		if code != exitOK || stdout != "siding-proxy already runs on http://*.localhost/, memory budget 12.0 GiB\n" {
			t.Errorf("exit %d, stdout %q", code, stdout)
		}
	})

	t.Run("keeps the running port without --port", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t)
		setupDoc(t, e, "--port", "8080")
		if doc := setupDoc(t, e); doc.State != "unchanged" || doc.Port != 8080 {
			t.Errorf("%+v", doc)
		}
	})

	t.Run("another port recreates the proxy", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t)
		setupDoc(t, e)
		if doc := setupDoc(t, e, "--port", "8080"); doc.State != "updated" || doc.Port != 8080 || doc.URL != "http://*.localhost:8080/" {
			t.Errorf("%+v", doc)
		}
	})

	t.Run("budget", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t)
		if doc := setupDoc(t, e, "--budget", "12g"); doc.BudgetBytes != 12<<30 {
			t.Errorf("set: %+v", doc)
		}
		if got := e.docker.proxy.Labels["dev.siding.budget"]; got != "12884901888" {
			t.Errorf("label %q", got)
		}
		if doc := setupDoc(t, e); doc.State != "unchanged" || doc.BudgetBytes != 12<<30 {
			t.Errorf("kept: %+v", doc)
		}
		e.docker.memTotal = 8 << 30
		if doc := setupDoc(t, e, "--budget", "default"); doc.State != "updated" || doc.BudgetBytes != 6<<30 {
			t.Errorf("default: %+v", doc)
		}
		if code, stdout, _ := e.run("setup", "--budget", "lots", "--json"); code != exitFailed || !strings.Contains(stdout, `"code": "InvalidArguments"`) {
			t.Errorf("bad size: exit %d, %s", code, stdout)
		}
	})

	t.Run("a proxy of siding v0.1", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t)
		e.proxyUp(8080) // created by v0.1: no compose file of this siding
		if doc := setupDoc(t, e); doc.State != "updated" || doc.Port != 8080 {
			t.Errorf("%+v", doc)
		}
	})

	t.Run("route to the overview", func(t *testing.T) {
		t.Parallel()
		for _, tt := range []struct {
			name  string
			local bool
			alias string
		}{
			{name: "daemon on this machine", local: true, alias: "172.19.0.1"},
			{name: "daemon in a VM", local: false, alias: "host-gateway"},
		} {
			e := newEnv(t)
			e.opts.IsLocalAddress = func(string) bool { return tt.local }
			setupDoc(t, e)
			data, err := os.ReadFile(filepath.Join(e.opts.Home, ".siding", "proxy", "siding", "compose.yml"))
			if err != nil {
				t.Fatal(err)
			}
			routes := filepath.Join(e.opts.Home, ".siding", "proxy", "siding", "routes")
			for _, want := range []string{"--providers.file.directory=/etc/siding/routes", "--providers.file.watch=true",
				`"host.docker.internal:` + tt.alias + `"`, `"` + routes + `:/etc/siding/routes:ro"`, `dev.siding.host-alias: "` + tt.alias + `"`} {
				if !strings.Contains(string(data), want) {
					t.Errorf("%s: the compose file lacks %s", tt.name, want)
				}
			}
			if fi, err := os.Stat(routes); err != nil || !fi.IsDir() {
				t.Errorf("%s: no routes directory: %v", tt.name, err)
			}
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
