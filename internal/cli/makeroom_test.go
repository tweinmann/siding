package cli

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/tweinmann/siding/internal/proxy"
	"github.com/tweinmann/siding/internal/stack"
)

// roomEnv: a 4 GiB budget, a proxy that records requests and started two hours ago, and two
// stacks of the repository blog that started an hour ago: blog-main with 1 GiB, last requested
// 20 minutes ago, and blog-fix with 2.5 GiB, last requested a minute ago. This stack's
// estimate is 1 GiB (four services of 256 MiB), so it fits only without blog-main.
func roomEnv(t *testing.T) *env {
	t.Helper()
	const mib = 1 << 20
	e := newEnv(t)
	now := e.clock.now
	e.proxyUp(80)
	e.docker.proxy.Labels[stack.LabelBudget] = fmt.Sprint(4096 * mib)
	e.docker.proxy.Cmd = []string{"--ping=true", proxy.AccessLogFlag}
	e.docker.proxy.StartedAt = now.Add(-2 * time.Hour)
	for _, s := range []struct {
		branch string
		memory uint64
	}{{"main", 1024 * mib}, {"fix", 2560 * mib}} {
		id := e.id()
		id.Repo, id.Branch, id.Worktree = "blog", s.branch, "/src/blog-"+s.branch
		c := container(id, "web", "running", "", 0)
		c.StartedAt = now.Add(-time.Hour)
		e.docker.add(c)
		e.docker.memory[c.ID] = s.memory
	}
	e.request("blog-main-web-route", now.Add(-20*time.Minute))
	e.request("blog-fix-web-route", now.Add(-time.Minute))
	e.compose.onRun = func(r composeRun) {
		switch {
		case r.args[0] == "up" && r.inv.Project == "shop-main":
			e.stackUp(e.id(), "api", "db", "mail", "web")
		case r.args[0] == "stop":
			e.docker.stopProject(r.inv.Project)
		}
	}
	return e
}

// request writes an access log line of the proxy, as Traefik does.
func (e *env) request(router string, at time.Time) {
	e.docker.mu.Lock()
	defer e.docker.mu.Unlock()
	id := e.docker.proxy.ID
	e.docker.stdout[id] = append(e.docker.stdout[id],
		fmt.Sprintf(`{"RouterName":"%s@docker","StartUTC":"%s","level":"info","msg":"","time":"%s"}`, router, at.Format(time.RFC3339Nano), at.Format(time.RFC3339)))
}

// stopProject does what docker compose stop does: every container of the project exits.
func (d *fakeDocker) stopProject(project string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for i := range d.containers {
		if d.containers[i].Labels[stack.LabelProject] == project {
			d.containers[i].State, d.containers[i].ExitCode = "exited", 137
		}
	}
}

func (e *env) stateOf(project string) string {
	e.docker.mu.Lock()
	defer e.docker.mu.Unlock()
	for _, c := range e.docker.containers {
		if c.Labels[stack.LabelProject] == project {
			return c.State
		}
	}
	return "missing"
}

func (e *env) stops() []string {
	var out []string
	for _, r := range e.compose.recorded() {
		if r.args[0] == "stop" {
			out = append(out, r.inv.Project)
		}
	}
	return out
}

func TestMakeRoom(t *testing.T) {
	t.Parallel()

	t.Run("room from an idle stack", func(t *testing.T) {
		t.Parallel()
		e := roomEnv(t)
		code, stdout, stderr := e.run("up", "--wait", "--json")
		if code != exitOK || verdictOf(t, stdout) != "Ready" {
			t.Fatalf("exit %d, stderr %q\n%s", code, stderr, stdout)
		}
		if want := "Stopped idle stack blog-main (no request for 20m, 1.0 GiB) to make room for shop-main"; !strings.Contains(stderr, want) {
			t.Errorf("stderr %q lacks %q", stderr, want)
		}
		if strings.Contains(stderr, "Waiting for memory") {
			t.Errorf("it waited although room was made: %q", stderr)
		}
		if got := e.stops(); len(got) != 1 || got[0] != "blog-main" || e.stateOf("blog-fix") != "running" {
			t.Errorf("stopped %v, blog-fix %s", got, e.stateOf("blog-fix"))
		}
	})

	t.Run("never requested", func(t *testing.T) {
		t.Parallel()
		e := roomEnv(t)
		e.docker.stdout[e.docker.proxy.ID] = nil
		e.request("blog-fix-web", e.clock.now.Add(-time.Minute))
		// blog-main started an hour ago and was never requested since.
		code, _, stderr := e.run("up")
		if code != exitOK || !strings.Contains(stderr, "Stopped idle stack blog-main (no request for 1h0m") {
			t.Errorf("exit %d, stderr %q", code, stderr)
		}
	})

	t.Run("longest idle first, only as many as needed", func(t *testing.T) {
		t.Parallel()
		e := roomEnv(t)
		e.docker.stdout[e.docker.proxy.ID] = nil
		e.request("blog-main-web", e.clock.now.Add(-20*time.Minute))
		e.request("blog-fix-web", e.clock.now.Add(-40*time.Minute))
		// Both are idle; blog-fix is idle longer, and stopping it alone is enough.
		if code, _, stderr := e.run("up"); code != exitOK || len(e.stops()) != 1 || e.stops()[0] != "blog-fix" {
			t.Errorf("exit %d, stopped %v, stderr %q", code, e.stops(), stderr)
		}
	})

	t.Run("not enough even then", func(t *testing.T) {
		t.Parallel()
		e := roomEnv(t)
		e.docker.proxy.Labels[stack.LabelBudget] = fmt.Sprint(3 << 30)
		code, stdout, _ := e.run("up", "--json")
		if code != exitNoCapacity || !strings.Contains(stdout, `"code": "NoCapacity"`) || len(e.stops()) != 0 {
			t.Errorf("exit %d, stopped %v\n%s", code, e.stops(), stdout)
		}
	})

	t.Run("in line", func(t *testing.T) {
		t.Parallel()
		e := roomEnv(t)
		e.docker.stdout[e.docker.proxy.ID] = nil
		e.request("blog-main-web", e.clock.now.Add(-3*time.Minute))
		e.request("blog-fix-web", e.clock.now.Add(-time.Minute))
		code, stdout, stderr := e.run("up", "--wait", "--json")
		if code != exitOK || verdictOf(t, stdout) != "Ready" || !strings.Contains(stderr, "place 1 in line") ||
			!strings.Contains(stderr, "Stopped idle stack blog-main (no request for 5m") {
			t.Fatalf("exit %d, stderr %q\n%s", code, stderr, stdout)
		}
		// blog-main became idle after two minutes in line; the log was read every 30 seconds.
		if got := e.stops(); len(got) != 1 || got[0] != "blog-main" {
			t.Errorf("stopped %v", got)
		}
		if waited := e.clock.now.Sub(time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)); waited < 2*time.Minute || waited > 3*time.Minute {
			t.Errorf("waited %s", waited)
		}
	})

	t.Run("--force stops none", func(t *testing.T) {
		t.Parallel()
		e := roomEnv(t)
		if code, _, stderr := e.run("up", "--force"); code != exitOK || len(e.stops()) != 0 || !strings.Contains(stderr, "--force") {
			t.Errorf("exit %d, stopped %v, stderr %q", code, e.stops(), stderr)
		}
	})

	t.Run("--idle off stops none", func(t *testing.T) {
		t.Parallel()
		e := roomEnv(t)
		e.docker.proxy.Labels[stack.LabelIdle] = "0"
		if code, _, _ := e.run("up"); code != exitNoCapacity || len(e.stops()) != 0 {
			t.Errorf("exit %d, stopped %v", code, e.stops())
		}
	})

	t.Run("another threshold", func(t *testing.T) {
		t.Parallel()
		e := roomEnv(t)
		e.docker.proxy.Labels[stack.LabelIdle] = "1800" // 30m: blog-main's 20 minutes are not enough
		if code, _, _ := e.run("up"); code != exitNoCapacity || len(e.stops()) != 0 {
			t.Errorf("exit %d, stopped %v", code, e.stops())
		}
	})

	t.Run("a proxy without the record", func(t *testing.T) {
		t.Parallel()
		e := roomEnv(t)
		e.docker.proxy.Cmd = nil // created by siding v0.3
		code, _, stderr := e.run("up")
		if code != exitNoCapacity || len(e.stops()) != 0 || !strings.Contains(stderr, "siding setup turns it on") {
			t.Errorf("exit %d, stopped %v, stderr %q", code, e.stops(), stderr)
		}
	})

	t.Run("the proxy was recreated", func(t *testing.T) {
		t.Parallel()
		e := roomEnv(t)
		e.docker.stdout[e.docker.proxy.ID] = nil
		e.docker.proxy.StartedAt = e.clock.now.Add(-time.Minute)
		if code, _, _ := e.run("up"); code != exitNoCapacity || len(e.stops()) != 0 {
			t.Errorf("exit %d, stopped %v", code, e.stops())
		}
	})

	t.Run("never its own stack", func(t *testing.T) {
		t.Parallel()
		e := roomEnv(t)
		e.docker.stdout[e.docker.proxy.ID] = nil
		e.request("blog-main-web", e.clock.now.Add(-time.Minute))
		e.request("blog-fix-web", e.clock.now.Add(-time.Minute))
		// This worktree's stack runs, idle for an hour; its next up must not stop it.
		for _, s := range []string{"api", "db", "mail", "web"} {
			c := container(e.id(), s, "running", "", 0)
			c.StartedAt = e.clock.now.Add(-time.Hour)
			e.docker.add(c)
		}
		if code, _, _ := e.run("up"); code != exitNoCapacity || len(e.stops()) != 0 {
			t.Errorf("exit %d, stopped %v", code, e.stops())
		}
	})

	t.Run("stopped stacks use nothing", func(t *testing.T) {
		t.Parallel()
		e := roomEnv(t)
		// blog-fix held 2.5 GiB before it was stopped.
		e.docker.stopProject("blog-fix")
		if code, _, stderr := e.run("up"); code != exitOK || len(e.stops()) != 0 {
			t.Errorf("exit %d, stopped %v, stderr %q", code, e.stops(), stderr)
		}
	})
}

func TestIdleText(t *testing.T) {
	t.Parallel()
	for d, want := range map[time.Duration]string{
		45 * time.Second: "45s", 20*time.Minute + 30*time.Second: "20m", 2*time.Hour + 5*time.Minute: "2h5m",
	} {
		if got := idleText(d); got != want {
			t.Errorf("%s: %q, want %q", d, got, want)
		}
	}
}
