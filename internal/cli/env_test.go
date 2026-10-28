package cli

import (
	"bytes"
	"context"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tweinmann/siding/internal/compose"
	"github.com/tweinmann/siding/internal/docker"
	"github.com/tweinmann/siding/internal/git"
	"github.com/tweinmann/siding/internal/probe"
	"github.com/tweinmann/siding/internal/proxy"
	"github.com/tweinmann/siding/internal/stack"
)

// testConfig is what docker compose config would print for testCompose: web on / (one port),
// api on /api (a port label picks 8080 of two), db with no port, and mail with one exposed
// port but no route label.
const testConfig = `{
  "name": "shop-main",
  "services": {
    "api": {"expose": ["8080", "9229"], "labels": {"dev.siding.route": "/api", "dev.siding.port": "8080"},
            "networks": {"default": null}, "environment": {"API_TOKEN": "not-a-real-api-token"}},
    "db": {"networks": {"default": null}, "environment": {"POSTGRES_PASSWORD": "not-a-real-token"}},
    "mail": {"expose": ["8025"], "networks": {"default": null}},
    "web": {"ports": [{"target": 3000, "published": "3000", "protocol": "tcp"}], "labels": {"dev.siding.route": "/"},
            "networks": {"default": null}}
  },
  "networks": {"default": {"name": "shop-main_default"}},
  "volumes": {"data": {"name": "shop-main_data"}}
}`

// env is one test's machine: a real git repository <base>/shop on main with a compose file,
// and fakes for everything else.
type env struct {
	t       *testing.T
	base    string // parent of the repository, shown as /src in golden files
	repo    string
	opts    Options
	docker  *fakeDocker
	compose *fakeCompose
	probe   *fakeProbe
	browser *fakeBrowser
	clock   *fakeClock
	envVars map[string]string
}

func newEnv(t *testing.T) *env {
	t.Helper()
	// The real path: git reports it, and on macOS the temporary directory is a symlink.
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	e := &env{
		t:       t,
		base:    base,
		repo:    filepath.Join(base, "shop"),
		docker:  &fakeDocker{logs: map[string][]string{}, memory: map[string]uint64{}},
		compose: &fakeCompose{config: testConfig},
		probe:   &fakeProbe{answers: map[string]probe.Response{}},
		browser: &fakeBrowser{},
		clock:   &fakeClock{now: time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)},
		envVars: map[string]string{},
	}
	if err := os.MkdirAll(e.repo, 0o755); err != nil {
		t.Fatal(err)
	}
	e.git("init", "-q", "-b", "main")
	e.write("compose.yaml", "services: {}\n")
	e.write(".env", "POSTGRES_PASSWORD=not-a-real-token\n")
	e.git("add", ".")
	e.git("commit", "-q", "-m", "first")
	e.opts = Options{
		Build:       testBuild,
		Dir:         e.repo,
		Home:        filepath.Join(base, "home"),
		Getenv:      func(k string) string { return e.envVars[k] },
		OSMachineID: func() string { return "not-a-real-machine-id" },
		Git:         git.Client{},
		Compose:     e.compose,
		Docker:      e.docker,
		Probe:       e.probe,
		Browser:     e.browser,
		Clock:       e.clock,
		Proxy:       proxy.Default(),
	}
	return e
}

func (e *env) git(args ...string) {
	e.t.Helper()
	cmd := exec.Command("git", append([]string{"-C", e.repo}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
	if out, err := cmd.CombinedOutput(); err != nil {
		e.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func (e *env) write(name, content string) {
	e.t.Helper()
	path := filepath.Join(e.repo, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		e.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		e.t.Fatal(err)
	}
}

// run runs siding with args and returns exit code, stdout and stderr, with the temporary
// directory shown as /src.
func (e *env) run(args ...string) (code int, stdout, stderr string) {
	var out, errOut bytes.Buffer
	code = Execute(context.Background(), New(e.opts), args, &out, &errOut)
	return code, e.clean(out.String()), e.clean(errOut.String())
}

func (e *env) clean(s string) string { return strings.ReplaceAll(s, e.base, "/src") }

// id is the identity siding gives the test repository.
func (e *env) id() stack.Identity {
	machine, err := stack.MachineID("not-a-real-machine-id", "")
	if err != nil {
		e.t.Fatal(err)
	}
	return stack.Identity{Repo: "shop", Branch: "main", Worktree: e.repo, Machine: machine}
}

// proxyUp makes siding-proxy run on port.
func (e *env) proxyUp(port int) {
	e.docker.mu.Lock()
	defer e.docker.mu.Unlock()
	e.docker.proxy = &docker.Container{ID: "proxy", Name: "siding-proxy", Image: proxy.Image, State: "running",
		Labels: map[string]string{}, Ports: []docker.Port{{HostIP: "127.0.0.1", HostPort: port, ContainerPort: 80}}}
}

// stackUp adds one container per service of the test stack, running and, where routed,
// answering through the proxy.
func (e *env) stackUp(id stack.Identity, services ...string) {
	for _, s := range services {
		e.docker.add(container(id, s, "running", "", 0))
	}
	e.probe.set("/", id.Project()+"/web", 200)
	e.probe.set("/api", id.Project()+"/api", 404)
}

func container(id stack.Identity, service, state, health string, exit int) docker.Container {
	labels := id.ContainerLabels()
	labels[stack.LabelProject] = id.Project()
	labels[stack.LabelService] = service
	return docker.Container{ID: id.Project() + "-" + service + "-1", Name: id.Project() + "-" + service + "-1",
		Labels: labels, State: state, Health: health, ExitCode: exit}
}

type fakeDocker struct {
	mu         sync.Mutex
	pingErr    error
	proxy      *docker.Container
	containers []docker.Container
	networks   []docker.Network
	volumes    []docker.Volume
	logs       map[string][]string
	memory     map[string]uint64
	ensured    []string
}

func (d *fakeDocker) add(c docker.Container) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.containers = append(d.containers, c)
}

// update changes the container of a service.
func (d *fakeDocker) update(service string, f func(*docker.Container)) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for i := range d.containers {
		if d.containers[i].Labels[stack.LabelService] == service {
			f(&d.containers[i])
		}
	}
}

func matches(have, want map[string]string) bool {
	for k, v := range want {
		got, ok := have[k]
		if !ok || (v != "" && got != v) {
			return false
		}
	}
	return true
}

func (d *fakeDocker) Ping(context.Context) error { return d.pingErr }

func (d *fakeDocker) Containers(_ context.Context, labels map[string]string) ([]docker.Container, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	var out []docker.Container
	for _, c := range d.containers {
		if matches(c.Labels, labels) {
			c.Labels = maps.Clone(c.Labels)
			out = append(out, c)
		}
	}
	return out, nil
}

func (d *fakeDocker) Container(_ context.Context, name string) (*docker.Container, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.proxy != nil && d.proxy.Name == name {
		c := *d.proxy
		return &c, nil
	}
	return nil, nil
}

func (d *fakeDocker) Networks(_ context.Context, labels map[string]string) ([]docker.Network, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	var out []docker.Network
	for _, n := range d.networks {
		if matches(n.Labels, labels) {
			out = append(out, n)
		}
	}
	return out, nil
}

func (d *fakeDocker) Volumes(_ context.Context, labels map[string]string) ([]docker.Volume, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	var out []docker.Volume
	for _, v := range d.volumes {
		if matches(v.Labels, labels) {
			out = append(out, v)
		}
	}
	return out, nil
}

func (d *fakeDocker) EnsureNetwork(_ context.Context, name string, _ map[string]string) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.ensured = append(d.ensured, name)
	return true, nil
}

func (d *fakeDocker) Logs(_ context.Context, id string, tail int) ([]string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	lines := d.logs[id]
	if len(lines) > tail {
		lines = lines[len(lines)-tail:]
	}
	return lines, nil
}

func (d *fakeDocker) Memory(_ context.Context, id string) (uint64, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.memory[id], nil
}

type composeRun struct {
	inv  compose.Invocation
	args []string
}

type fakeCompose struct {
	mu     sync.Mutex
	config string
	runs   []composeRun
	runErr error
	// onRun, if set, runs after each recorded run, e.g. to start containers.
	onRun func(composeRun)
}

func (c *fakeCompose) Config(context.Context, compose.Invocation) ([]byte, error) {
	return []byte(c.config), nil
}

func (c *fakeCompose) Run(_ context.Context, inv compose.Invocation, args []string, _, _ io.Writer) error {
	c.mu.Lock()
	r := composeRun{inv: inv, args: args}
	c.runs = append(c.runs, r)
	onRun, err := c.onRun, c.runErr
	c.mu.Unlock()
	if err != nil {
		return err
	}
	if onRun != nil {
		onRun(r)
	}
	return nil
}

func (c *fakeCompose) recorded() []composeRun {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]composeRun(nil), c.runs...)
}

type fakeProbe struct {
	mu      sync.Mutex
	answers map[string]probe.Response // by path
	asked   []string
}

func (p *fakeProbe) set(path, service string, status int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.answers[path] = probe.Response{Status: status, Service: service}
}

func (p *fakeProbe) Get(_ context.Context, addr, host, path string) (probe.Response, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.asked = append(p.asked, addr+" "+host+path)
	if r, ok := p.answers[path]; ok {
		return r, nil
	}
	return probe.Response{Status: 404}, nil // like Traefik without a router
}

type fakeBrowser struct {
	mu     sync.Mutex
	opened []string
}

func (b *fakeBrowser) Open(url string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.opened = append(b.opened, url)
	return nil
}

// fakeClock advances only when slept on, and runs onSleep after each sleep.
type fakeClock struct {
	mu      sync.Mutex
	now     time.Time
	slept   int
	onSleep func(slept int)
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Sleep(_ context.Context, d time.Duration) error {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.slept++
	n, f := c.slept, c.onSleep
	c.mu.Unlock()
	if f != nil {
		f(n)
	}
	return nil
}
