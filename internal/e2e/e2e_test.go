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
	"sync/atomic"
	"testing"
	"time"

	"github.com/tweinmann/siding/internal/cli"
	"github.com/tweinmann/siding/internal/compose"
	"github.com/tweinmann/siding/internal/docker"
	"github.com/tweinmann/siding/internal/git"
	"github.com/tweinmann/siding/internal/overview"
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
	// pids gives every siding run its own pid in the queue: they all share this process.
	pids *atomic.Int32
}

func newMachine(t *testing.T) *machine {
	t.Helper()
	m := &machine{t: t, home: t.TempDir(), pids: &atomic.Int32{}}
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
	return m.sidingCtx(context.Background(), dir, args...)
}

func (m *machine) sidingCtx(ctx context.Context, dir string, args ...string) (int, string) {
	m.t.Helper()
	code, stdout, _ := m.sidingFull(ctx, dir, args...)
	return code, stdout
}

// sidingFull is siding with stderr too.
func (m *machine) sidingFull(ctx context.Context, dir string, args ...string) (int, string, string) {
	m.t.Helper()
	// A machine id of its own: gc acts on every stack of "this machine", and must only ever see
	// the tests' stacks, never one a developer started in the same daemon.
	opts := cli.Options{
		Dir: dir, Home: m.home, Getenv: os.Getenv, OSMachineID: func() string { return "siding-test-e2e" },
		IsLocalAddress: overview.IsLocalAddress, Stdin: strings.NewReader(""), IsTerminal: func() bool { return false },
		PID: os.Getpid() + int(m.pids.Add(1))*1000000, Alive: func(int) bool { return true },
		Git: git.Client{}, Compose: &compose.CLI{}, Docker: &docker.Client{},
		Probe: probe.HTTP{Header: stack.ServiceHeader}, Browser: noBrowser{}, Clock: cli.SystemClock{},
		Proxy: testProxy,
	}
	var out, errOut bytes.Buffer
	code := cli.Execute(ctx, cli.New(opts), args, &out, &errOut)
	m.t.Logf("siding %s: exit %d\n%s", strings.Join(args, " "), code, errOut.String())
	return code, out.String(), errOut.String()
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
		// Only the data is left; down --volumes still finds the stack by its volumes.
		if code, out := m.siding(shop, "down", "--volumes", "--force", "--json"); code != 0 || !strings.Contains(out, "siding-test-shop-feature-x_data") {
			t.Errorf("down --volumes after down: exit %d\n%s", code, out)
		}
		if out := run(t, "", "docker", "volume", "ls", "-q", "--filter", "name=siding-test-shop-feature-x_data"); strings.TrimSpace(out) != "" {
			t.Errorf("the volume survived: %q", out)
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

	t.Run("down --volumes after a squash merge", func(t *testing.T) {
		squash := repo(t, "squash", shopCompose, "", "feature/s")
		origin := filepath.Join(t.TempDir(), "origin.git")
		gitIn(t, squash, "init", "-q", "--bare", origin)
		gitIn(t, squash, "remote", "add", "origin", origin)
		gitIn(t, squash, "push", "-q", "-u", "origin", "feature/s")
		gitIn(t, squash, "push", "-q", "origin", "--delete", "feature/s")
		if code, out := m.siding(squash, "up", "--repo", "siding-test-squash", "--wait", "--json"); code != 0 {
			t.Fatalf("up: exit %d\n%s", code, out)
		}
		if code, out := m.siding(squash, "down", "--volumes", "--json"); code != 0 || !strings.Contains(out, "siding-test-squash-feature-s_data") {
			t.Errorf("exit %d\n%s", code, out)
		}
	})

	t.Run("budget", func(t *testing.T) {
		budgeted := repo(t, "budget", shopCompose, "", "feature/b")
		// The stack's estimate is 512 MiB (two services without limits).
		if code, out := m.siding(t.TempDir(), "setup", "--budget", "64m", "--json"); code != 0 {
			t.Fatalf("setup --budget: exit %d\n%s", code, out)
		}
		// With --wait it waits in line; a short timeout ends it with exit 3 as well.
		code, out := m.siding(budgeted, "up", "--repo", "siding-test-budget", "--wait", "--timeout", "5s", "--json")
		if code != 3 || decode(t, out).Code != "NoCapacity" {
			t.Errorf("over budget: exit %d\n%s", code, out)
		}
		if out := run(t, "", "docker", "ps", "-aq", "--filter", "label=com.docker.compose.project=siding-test-budget-feature-b"); strings.TrimSpace(out) != "" {
			t.Errorf("up started containers although it refused: %q", out)
		}
		if code, out := m.siding(budgeted, "up", "--repo", "siding-test-budget", "--force", "--wait", "--json"); code != 0 {
			t.Errorf("--force: exit %d\n%s", code, out)
		}
		if code, out := m.siding(t.TempDir(), "setup", "--budget", "default", "--json"); code != 0 {
			t.Fatalf("setup --budget default: exit %d\n%s", code, out)
		}
	})

	t.Run("gc", func(t *testing.T) {
		merged := repo(t, "gcm", shopCompose, "", "feature/m")
		unmerged := repo(t, "gcu", shopCompose, "", "feature/u")
		for dir, name := range map[string]string{merged: "siding-test-gcm", unmerged: "siding-test-gcu"} {
			if code, out := m.siding(dir, "up", "--repo", name, "--wait", "--json"); code != 0 {
				t.Fatalf("up %s: exit %d\n%s", name, code, out)
			}
		}
		// Merge feature/m into main, move main on, and go back: a clean, merged worktree.
		gitIn(t, merged, "switch", "-q", "main")
		gitIn(t, merged, "merge", "-q", "--ff-only", "feature/m")
		gitIn(t, merged, "commit", "-q", "--allow-empty", "-m", "after the merge")
		gitIn(t, merged, "switch", "-q", "feature/m")

		code, out := m.siding(t.TempDir(), "gc", "--dry-run", "--json")
		if code != 0 || !strings.Contains(out, `"project": "siding-test-gcm-feature-m"`) || !strings.Contains(out, `"reason": "Merged"`) {
			t.Fatalf("dry run: exit %d\n%s", code, out)
		}
		if !strings.Contains(out, `"project": "siding-test-gcu-feature-u"`) || !strings.Contains(out, `"reason": "Unmerged"`) {
			t.Errorf("the unmerged stack is not kept: %s", out)
		}
		if code, out := m.siding(t.TempDir(), "gc", "--json"); code != 1 || decode(t, out).Code != "NotInteractive" {
			t.Errorf("without --yes: exit %d\n%s", code, out)
		}
		if code, out := m.siding(t.TempDir(), "gc", "--yes", "--json"); code != 0 || !strings.Contains(out, "siding-test-gcm-feature-m_data") {
			t.Fatalf("--yes: exit %d\n%s", code, out)
		}
		if out := run(t, "", "docker", "volume", "ls", "-q", "--filter", "name=siding-test-gcm-feature-m_data"); strings.TrimSpace(out) != "" {
			t.Errorf("the merged stack's volume survived: %q", out)
		}
		if _, err := os.Stat(merged); err != nil {
			t.Errorf("gc removed the worktree: %v", err)
		}
		if out := run(t, "", "docker", "ps", "-q", "--filter", "label=com.docker.compose.project=siding-test-gcu-feature-u"); strings.TrimSpace(out) == "" {
			t.Error("gc removed the unmerged stack")
		}
	})

	t.Run("ui", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan int, 1)
		go func() {
			code, _ := m.sidingCtx(ctx, t.TempDir(), "ui")
			done <- code
		}()
		var code int
		for i := 0; i < 40 && code != 200; i++ {
			time.Sleep(250 * time.Millisecond)
			code, _, _ = get(t, "siding.localhost", "/api/stacks")
		}
		if code != 200 {
			t.Errorf("siding.localhost through the proxy: %d", code)
		}
		cancel()
		if got := <-done; got != 130 {
			t.Errorf("ui ended with %d, want 130", got)
		}
		for i := 0; i < 40 && code != 404; i++ {
			time.Sleep(250 * time.Millisecond)
			code, _, _ = get(t, "siding.localhost", "/")
		}
		if code != 404 {
			t.Errorf("after ui stopped: %d, want 404", code)
		}
	})

	t.Run("teardown", func(t *testing.T) {
		kept := repo(t, "keep", shopCompose, "", "feature/k")
		if code, out := m.siding(kept, "up", "--repo", "siding-test-keep", "--wait", "--json"); code != 0 {
			t.Fatalf("up: exit %d\n%s", code, out)
		}
		code, out := m.siding(t.TempDir(), "teardown", "--json")
		if code != 0 || !strings.Contains(out, `"keptNetwork": "siding-test-e2e"`) || !strings.Contains(out, "siding-test-keep-feature-k") {
			t.Fatalf("with a stack: exit %d\n%s", code, out)
		}
		if out := run(t, "", "docker", "ps", "-aq", "--filter", "name=^siding-test-e2e-proxy$"); strings.TrimSpace(out) != "" {
			t.Errorf("the proxy survived: %q", out)
		}
		if out := run(t, "", "docker", "ps", "-q", "--filter", "label=com.docker.compose.project=siding-test-keep-feature-k"); strings.TrimSpace(out) == "" {
			t.Error("teardown stopped the stack")
		}
		if code, out := m.siding(kept, "down", "--json"); code != 0 {
			t.Fatalf("down: exit %d\n%s", code, out)
		}
		if code, out := m.siding(t.TempDir(), "teardown", "--json"); code != 0 || !strings.Contains(out, `"network": "siding-test-e2e"`) {
			t.Fatalf("without a stack: exit %d\n%s", code, out)
		}
		if out := run(t, "", "docker", "network", "ls", "-q", "--filter", "name=^siding-test-e2e$"); strings.TrimSpace(out) != "" {
			t.Errorf("the network survived: %q", out)
		}
	})
}
