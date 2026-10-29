package cli

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tweinmann/siding/internal/docker"
	"github.com/tweinmann/siding/internal/fail"
	"github.com/tweinmann/siding/internal/testutil"
)

func TestStatus(t *testing.T) {
	t.Parallel()

	t.Run("ready", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t)
		e.proxyUp(80)
		e.stackUp(e.id(), "api", "mail", "web")
		e.docker.add(container(e.id(), "db", "running", "healthy", 0))
		code, stdout, stderr := e.run("status", "--json")
		if code != exitOK || stderr != "" {
			t.Fatalf("exit %d, stderr %q, stdout %s", code, stderr, stdout)
		}
		testutil.Golden(t, filepath.Join("testdata", "json", "status-ready.json"), []byte(stdout))
		if !slices.Contains(e.probe.asked, "127.0.0.1:80 shop.main.localhost/api") {
			t.Errorf("probes %v", e.probe.asked)
		}
	})

	t.Run("stopped", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t)
		e.proxyUp(80)
		for _, svc := range []string{"api", "db", "mail", "web"} {
			e.docker.add(container(e.id(), svc, "exited", "", 137))
		}
		// --wait ends at once: a stopped stack does not start by itself.
		code, stdout, stderr := e.run("status", "--wait", "--json")
		if code != exitTimedOut || stderr != "" {
			t.Fatalf("exit %d, stderr %q, stdout %s", code, stderr, stdout)
		}
		testutil.Golden(t, filepath.Join("testdata", "json", "status-stopped.json"), []byte(stdout))
		if len(e.probe.asked) != 0 || e.clock.slept != 0 {
			t.Errorf("probes %v, slept %d", e.probe.asked, e.clock.slept)
		}
		if code, out, _ := e.run("status"); code != exitTimedOut || !strings.Contains(out, "shop-main: Stopped") || !strings.Contains(out, "siding up --wait starts it again") {
			t.Errorf("text: exit %d\n%s", code, out)
		}
	})

	t.Run("failed with the log tail, redacted", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t)
		e.proxyUp(80)
		e.stackUp(e.id(), "db", "mail", "web")
		api := container(e.id(), "api", "exited", "", 1)
		e.docker.add(api)
		e.docker.logs[api.ID] = []string{"starting", "connecting to postgres://app:not-a-real-token@db/app", "cannot connect to db"}
		code, stdout, _ := e.run("status", "--json")
		if code != exitFailed {
			t.Fatalf("exit %d, stdout %s", code, stdout)
		}
		testutil.Golden(t, filepath.Join("testdata", "json", "status-failed.json"), []byte(stdout))
		if strings.Contains(stdout, "not-a-real-token") {
			t.Error("a .env value reached the document")
		}
	})

	t.Run("text of a failure", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t)
		e.proxyUp(80)
		e.stackUp(e.id(), "api", "mail", "web")
		db := container(e.id(), "db", "running", "unhealthy", 0)
		db.HealthOutput = "pg_isready: no response"
		e.docker.add(db)
		e.docker.logs[db.ID] = []string{"FATAL: password authentication failed"}
		code, stdout, _ := e.run("status")
		want := `shop-main: Failed  http://shop.main.localhost/
  api   running              answering  http://api.shop.main.localhost/  route /api
  db    running (unhealthy)
  mail  running                         http://mail.shop.main.localhost/
  web   running              answering  http://web.shop.main.localhost/  route /

db: Unhealthy, its healthcheck fails: pg_isready: no response
last log lines of db:
  | FATAL: password authentication failed
`
		if code != exitFailed || stdout != want {
			t.Errorf("exit %d, stdout:\n%s\nwant:\n%s", code, stdout, want)
		}
	})

	t.Run("timed out", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t)
		e.proxyUp(80)
		e.stackUp(e.id(), "api", "db", "mail", "web")
		delete(e.probe.answers, "/") // web runs but never answers
		code, stdout, _ := e.run("status", "--wait", "--timeout", "10s", "--json")
		if code != exitTimedOut {
			t.Fatalf("exit %d, stdout %s", code, stdout)
		}
		testutil.Golden(t, filepath.Join("testdata", "json", "status-timedout.json"), []byte(stdout))
		if e.clock.slept != 10 {
			t.Errorf("slept %d times, want 10 one-second polls", e.clock.slept)
		}
	})

	t.Run("waits until ready", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t)
		e.proxyUp(80)
		e.stackUp(e.id(), "api", "mail", "web")
		e.docker.add(container(e.id(), "db", "running", "starting", 0))
		e.clock.onSleep = func(n int) {
			if n == 3 {
				e.docker.update("db", func(c *docker.Container) { c.Health = "healthy" })
			}
		}
		code, stdout, _ := e.run("status", "--wait", "--json")
		if code != exitOK || verdictOf(t, stdout) != "Ready" || e.clock.slept != 3 {
			t.Errorf("exit %d, verdict %s, slept %d", code, verdictOf(t, stdout), e.clock.slept)
		}
	})

	t.Run("starting without wait", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t)
		e.proxyUp(80)
		e.stackUp(e.id(), "api", "mail", "web")
		e.docker.add(container(e.id(), "db", "running", "starting", 0))
		code, stdout, _ := e.run("status", "--json")
		if code != exitTimedOut || verdictOf(t, stdout) != "Starting" || e.clock.slept != 0 {
			t.Errorf("exit %d, verdict %s", code, verdictOf(t, stdout))
		}
	})

	for _, tt := range []struct {
		name  string
		setup func(e *env)
		code  string
	}{
		{name: "no stack", setup: func(e *env) { e.proxyUp(80) }, code: fail.StackNotFound},
		{name: "no proxy", setup: func(e *env) { e.stackUp(e.id(), "web") }, code: fail.ProxyNotRunning},
		{name: "another branch's stack", setup: func(e *env) {
			e.proxyUp(80)
			other := e.id()
			other.Branch = "feature/login"
			e.stackUp(other, "web")
		}, code: fail.StackNotFound},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := newEnv(t)
			tt.setup(e)
			if code, stdout, _ := e.run("status", "--json"); code != exitFailed || !strings.Contains(stdout, `"code": "`+tt.code+`"`) {
				t.Errorf("exit %d, stdout %s", code, stdout)
			}
		})
	}
}

func TestUpWait(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.proxyUp(80)
	e.compose.onRun = func(composeRun) {
		e.stackUp(e.id(), "api", "db", "mail", "web")
	}
	code, stdout, stderr := e.run("up", "--wait", "--json")
	if code != exitOK || verdictOf(t, stdout) != "Ready" || stderr != "" {
		t.Errorf("exit %d, stdout %s, stderr %q", code, stdout, stderr)
	}

	e.docker.update("api", func(c *docker.Container) { c.State, c.ExitCode = "exited", 1 })
	if code, stdout, _ := e.run("up", "--wait", "--json"); code != exitFailed || verdictOf(t, stdout) != "Failed" {
		t.Errorf("after a crash: exit %d, verdict %s", code, verdictOf(t, stdout))
	}
}

// verdictOf reads the verdict of one Status document, and fails the test unless stdout is
// exactly one document.
func verdictOf(t *testing.T, stdout string) string {
	t.Helper()
	var doc statusDocument
	dec := json.NewDecoder(strings.NewReader(stdout))
	if err := dec.Decode(&doc); err != nil || dec.More() || doc.Kind != kindStatus {
		t.Fatalf("not one Status document (%v): %s", err, stdout)
	}
	return doc.Verdict
}
