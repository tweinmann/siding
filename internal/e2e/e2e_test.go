//go:build docker

// Package e2e runs siding's commands end to end against a real Docker daemon: the
// devcontainer's inner one, through just smoke-stacks. Everything it creates is named
// siding-test-* and removed by exact name; a developer's siding-proxy and network siding are
// never touched.
package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tweinmann/siding/internal/cli"
	"github.com/tweinmann/siding/internal/compose"
	"github.com/tweinmann/siding/internal/docker"
	"github.com/tweinmann/siding/internal/git"
	"github.com/tweinmann/siding/internal/probe"
	"github.com/tweinmann/siding/internal/proxy"
	"github.com/tweinmann/siding/internal/stack"
)

const (
	proxyPort = 18080
	addr      = "127.0.0.1:18080"
)

var testProxy = proxy.Config{
	Project:     "siding-test-e2e-proxy",
	Container:   "siding-test-e2e-proxy",
	Network:     "siding-test-e2e",
	Image:       proxy.Image,
	DefaultPort: proxyPort,
}

// A busybox httpd per service answers with the service's name at / and at /api.
const shopCompose = `services:
  web:
    image: busybox:1.37.0
    command: ["sh", "-c", "mkdir -p /www && echo web > /www/index.html && exec httpd -f -p 80 -h /www"]
    ports: ["18081:80"]
    labels: {dev.siding.route: "/"}
    volumes: [data:/data]
    healthcheck: {test: ["CMD", "wget", "-q", "-O", "/dev/null", "http://127.0.0.1/"], interval: 1s, retries: 30}
  api:
    image: busybox:1.37.0
    command: ["sh", "-c", "mkdir -p /www && echo api > /www/api && exec httpd -f -p 80 -h /www"]
    expose: ["80"]
    labels: {dev.siding.route: "/api"}
volumes:
  data: {}
`

// api crashes after logging a secret from .env; dead is routed but nothing listens on its port.
const brokenCompose = `services:
  api:
    image: busybox:1.37.0
    command: ["sh", "-c", "echo using token $$API_TOKEN; echo cannot connect to db; exit 1"]
    environment: {API_TOKEN: "${API_TOKEN}"}
    labels: {dev.siding.route: "/api"}
    expose: ["80"]
  dead:
    image: busybox:1.37.0
    command: ["sleep", "600"]
    expose: ["8081"]
    labels: {dev.siding.route: "/"}
`

type machine struct {
	t    *testing.T
	home string
}

func newMachine(t *testing.T) *machine {
	t.Helper()
	m := &machine{t: t, home: t.TempDir()}
	t.Cleanup(func() {
		// By exact name only.
		run(t, "", "docker", "compose", "-p", testProxy.Project, "down", "-v", "--remove-orphans")
		run(t, "", "docker", "network", "rm", testProxy.Network)
	})
	return m
}

// siding runs one command in dir and returns exit code and stdout; stderr goes to the log.
func (m *machine) siding(dir string, args ...string) (int, string) {
	m.t.Helper()
	opts := cli.Options{
		Dir: dir, Home: m.home, Getenv: os.Getenv, OSMachineID: stack.OSMachineID,
		Git: git.Client{}, Compose: &compose.CLI{}, Docker: &docker.Client{},
		Probe: probe.HTTP{Header: stack.ServiceHeader}, Browser: noBrowser{}, Clock: cli.SystemClock{},
		Proxy: testProxy,
	}
	var out, errOut bytes.Buffer
	code := cli.Execute(context.Background(), cli.New(opts), args, &out, &errOut)
	m.t.Logf("siding %s: exit %d\n%s", strings.Join(args, " "), code, errOut.String())
	return code, out.String()
}

type noBrowser struct{}

func (noBrowser) Open(string) error { return nil }

// repo creates a repository with a compose file on branch, one commit ahead of main, and
// removes its stack (project siding-test-<name>-<branch slug>) by exact name afterwards.
func repo(t *testing.T, name, composeFile, env, branch string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	gitIn(t, dir, "init", "-q", "-b", "main")
	gitIn(t, dir, "commit", "-q", "--allow-empty", "-m", "first")
	gitIn(t, dir, "switch", "-q", "-c", branch)
	for file, content := range map[string]string{"compose.yaml": composeFile, ".env": env} {
		if err := os.WriteFile(filepath.Join(dir, file), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gitIn(t, dir, "add", ".")
	gitIn(t, dir, "commit", "-q", "-m", "app")
	project := "siding-test-" + name + "-" + stack.Slug(branch)
	t.Cleanup(func() { run(t, "", "docker", "compose", "-p", project, "down", "-v", "--remove-orphans") })
	return dir
}

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// run runs a cleanup command and logs a failure instead of failing: the object may be gone.
func run(t *testing.T, dir string, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("%s %v: %v\n%s", name, args, err, out)
	}
	return string(out)
}

func get(t *testing.T, host, path string) (int, string, string) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, "http://"+addr+path, nil)
	req.Host = host
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("GET %s%s: %v", host, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, strings.TrimSpace(string(body)), resp.Header.Get(stack.ServiceHeader)
}

type status struct {
	Kind    string `json:"kind"`
	Verdict string `json:"verdict"`
	Code    string `json:"code"`
	Failure struct {
		Service, Reason, Hint string
		Logs                  []string
	} `json:"failure"`
}

func decode(t *testing.T, stdout string) status {
	t.Helper()
	var s status
	if err := json.Unmarshal([]byte(stdout), &s); err != nil {
		t.Fatalf("not a document: %v\n%s", err, stdout)
	}
	return s
}

func TestStacks(t *testing.T) {
	m := newMachine(t)

	// setup, twice.
	if code, out := m.siding(t.TempDir(), "setup", "--json"); code != 0 || !strings.Contains(out, `"state": "created"`) {
		t.Fatalf("setup: exit %d\n%s", code, out)
	}
	if code, out := m.siding(t.TempDir(), "setup", "--json"); code != 0 || !strings.Contains(out, `"state": "unchanged"`) {
		t.Fatalf("second setup: exit %d\n%s", code, out)
	}

	shop := repo(t, "shop", shopCompose, "", "feature/x")
	host := "siding-test-shop.feature-x.localhost"

	t.Run("up --wait is Ready", func(t *testing.T) {
		code, out := m.siding(shop, "up", "--repo", "siding-test-shop", "--wait", "--timeout", "90s", "--json")
		if s := decode(t, out); code != 0 || s.Verdict != "Ready" {
			t.Fatalf("exit %d\n%s", code, out)
		}
		if got := run(t, shop, "git", "status", "--porcelain"); got != "" {
			t.Errorf("the worktree is dirty after up: %q", got)
		}
	})

	t.Run("routing by host and path", func(t *testing.T) {
		for _, tt := range []struct {
			host, path, body, service string
		}{
			{host: host, path: "/", body: "web", service: "siding-test-shop-feature-x/web"},
			{host: host, path: "/api", body: "api", service: "siding-test-shop-feature-x/api"},
			{host: "web." + host, path: "/", body: "web", service: "siding-test-shop-feature-x/web"},
		} {
			code, body, service := get(t, tt.host, tt.path)
			if code != 200 || body != tt.body || service != tt.service {
				t.Errorf("%s%s: %d %q, %s %q; want 200 %q from %s", tt.host, tt.path, code, body, stack.ServiceHeader, service, tt.body, tt.service)
			}
		}
		// The app's own 404 carries the header; the proxy's 404 for an unknown host does not.
		if code, _, service := get(t, host, "/api/nope"); code != 404 || service == "" {
			t.Errorf("the app's 404: %d, header %q", code, service)
		}
		if code, _, service := get(t, "nope.localhost", "/"); code != 404 || service != "" {
			t.Errorf("unknown host: %d, header %q", code, service)
		}
	})

	t.Run("no published port", func(t *testing.T) {
		out := run(t, "", "docker", "ps", "--filter", "label=com.docker.compose.project=siding-test-shop-feature-x", "--format", "{{.Ports}}")
		if strings.Contains(out, "18081") || strings.Contains(out, "->") {
			t.Errorf("published ports: %q", out)
		}
	})

	t.Run("ps", func(t *testing.T) {
		code, out := m.siding(t.TempDir(), "ps", "--json")
		if code != 0 || !strings.Contains(out, `"project": "siding-test-shop-feature-x"`) || !strings.Contains(out, `"state": "running"`) {
			t.Errorf("exit %d\n%s", code, out)
		}
	})

	t.Run("a failure with its log tail", func(t *testing.T) {
		broken := repo(t, "broken", brokenCompose, "API_TOKEN=not-a-real-token\n", "main-work")
		code, out := m.siding(broken, "up", "--repo", "siding-test-broken", "--wait", "--timeout", "60s", "--json")
		s := decode(t, out)
		if code != 1 || s.Verdict != "Failed" || s.Failure.Service != "api" || s.Failure.Reason != "Exited" {
			t.Fatalf("exit %d\n%s", code, out)
		}
		logs := strings.Join(s.Failure.Logs, "\n")
		if !strings.Contains(logs, "cannot connect to db") || strings.Contains(out, "not-a-real-token") || !strings.Contains(logs, "using token ***") {
			t.Errorf("log tail %q", logs)
		}
		// Nothing listens behind dead's route: the proxy answers 502 on its own. up ended at
		// once with Failed, so give Traefik a moment to pick up dead's labels.
		var code502 int
		var service string
		for i := 0; i < 20 && code502 != 502; i++ {
			if i > 0 {
				time.Sleep(500 * time.Millisecond)
			}
			code502, _, service = get(t, "siding-test-broken.main-work.localhost", "/")
		}
		resp := probe.Response{Status: code502, Service: service}
		if code502 != 502 || resp.Answered("siding-test-broken-main-work/dead") {
			t.Errorf("dead: status %d, header %q; want 502, not answered", code502, service)
		}
		t.Logf("%s on Traefik's own 502: %q", stack.ServiceHeader, service)
	})

	t.Run("down keeps the data", func(t *testing.T) {
		if code, out := m.siding(shop, "down", "--json"); code != 0 || !strings.Contains(out, "siding-test-shop-feature-x-web-1") {
			t.Fatalf("exit %d\n%s", code, out)
		}
		if out := run(t, "", "docker", "volume", "ls", "-q", "--filter", "name=siding-test-shop-feature-x_data"); !strings.Contains(out, "siding-test-shop-feature-x_data") {
			t.Errorf("the volume is gone: %q", out)
		}
		if out := run(t, "", "docker", "ps", "-aq", "--filter", "label=com.docker.compose.project=siding-test-shop-feature-x"); strings.TrimSpace(out) != "" {
			t.Errorf("containers left: %q", out)
		}
	})

	t.Run("down --volumes keeps unmerged and dirty work", func(t *testing.T) {
		if code, out := m.siding(shop, "up", "--repo", "siding-test-shop", "--wait", "--json"); code != 0 {
			t.Fatalf("up again: exit %d\n%s", code, out)
		}
		if code, out := m.siding(shop, "down", "--volumes", "--json"); code != 1 || decode(t, out).Code != "UnmergedBranch" {
			t.Errorf("unmerged: exit %d\n%s", code, out)
		}
		if err := os.WriteFile(filepath.Join(shop, "notes.txt"), []byte("unsaved"), 0o644); err != nil {
			t.Fatal(err)
		}
		if code, out := m.siding(shop, "down", "--volumes", "--json"); code != 1 || decode(t, out).Code != "DirtyWorktree" {
			t.Errorf("dirty: exit %d\n%s", code, out)
		}
		if code, out := m.siding(shop, "down", "--volumes", "--force", "--json"); code != 0 || !strings.Contains(out, "siding-test-shop-feature-x_data") {
			t.Fatalf("forced: exit %d\n%s", code, out)
		}
		if out := run(t, "", "docker", "volume", "ls", "-q", "--filter", "name=siding-test-shop-feature-x_data"); strings.TrimSpace(out) != "" {
			t.Errorf("the volume survived --force: %q", out)
		}
	})
}
