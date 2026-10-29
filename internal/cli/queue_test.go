package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tweinmann/siding/internal/queue"
)

// queueEnv: a 4 GiB budget, another repository's stack using 3.5 GiB, and this stack's estimate
// of 1 GiB: it does not fit until the other stack shrinks.
func queueEnv(t *testing.T) *env {
	t.Helper()
	const mib = 1 << 20
	e := newEnv(t)
	e.proxyUp(80)
	e.docker.proxy.Labels["dev.siding.budget"] = fmt.Sprint(4096 * mib)
	other := e.id()
	other.Repo, other.Branch, other.Worktree = "blog", "main", "/src/blog"
	e.stackUp(other, "web")
	e.docker.memory["blog-main-web-1"] = 3584 * mib
	e.compose.onRun = func(r composeRun) {
		if r.args[0] == "up" && r.inv.Project == "shop-main" {
			e.stackUp(e.id(), "api", "db", "mail", "web")
		}
	}
	return e
}

func (e *env) shrinkOther() {
	e.docker.mu.Lock()
	defer e.docker.mu.Unlock()
	e.docker.memory["blog-main-web-1"] = 1000 << 20
}

func TestQueue(t *testing.T) {
	t.Parallel()

	t.Run("memory frees", func(t *testing.T) {
		t.Parallel()
		e := queueEnv(t)
		e.clock.onSleep = func(n int) {
			if n == 2 {
				e.shrinkOther()
			}
		}
		code, stdout, stderr := e.run("up", "--wait", "--json")
		if code != exitOK || verdictOf(t, stdout) != "Ready" || !strings.Contains(stderr, "Waiting for memory: place 1 in line") {
			t.Errorf("exit %d, stderr %q\n%s", code, stderr, stdout)
		}
		if files, _ := os.ReadDir(filepath.Join(e.opts.Home, ".siding", "queue")); len(files) != 0 {
			t.Errorf("still in line: %v", files)
		}
	})

	t.Run("time is up", func(t *testing.T) {
		t.Parallel()
		e := queueEnv(t)
		code, stdout, _ := e.run("up", "--wait", "--timeout", "10s", "--json")
		if code != exitNoCapacity || !strings.Contains(stdout, `"code": "NoCapacity"`) || !strings.Contains(stdout, "at place 1 in line") || len(e.compose.recorded()) != 0 {
			t.Errorf("exit %d, compose runs %d\n%s", code, len(e.compose.recorded()), stdout)
		}
		if files, _ := os.ReadDir(filepath.Join(e.opts.Home, ".siding", "queue")); len(files) != 0 {
			t.Errorf("still in line: %v", files)
		}
	})

	t.Run("first come, first served", func(t *testing.T) {
		t.Parallel()
		e := queueEnv(t)
		// Another up began to wait before this one.
		line := queue.Open(e.opts.Home, e.opts.Alive)
		if _, err := line.Enter(queue.Entry{Project: "big-main", PID: 1111, Estimate: 8 << 30, Since: e.clock.now.Add(-1)}); err != nil {
			t.Fatal(err)
		}
		e.clock.onSleep = func(n int) {
			switch n {
			case 1:
				e.shrinkOther() // this stack would fit now, but it is second
			case 3:
				if runs := len(e.compose.recorded()); runs != 0 {
					t.Errorf("started before the first in line: %d compose runs", runs)
				}
				e.dead[1111] = true // the first waiter was killed
			}
		}
		code, stdout, stderr := e.run("up", "--wait", "--json")
		if code != exitOK || verdictOf(t, stdout) != "Ready" || !strings.Contains(stderr, "place 2 in line") || !strings.Contains(stderr, "place 1 in line") {
			t.Errorf("exit %d, stderr %q", code, stderr)
		}
	})

	t.Run("without --wait it refuses", func(t *testing.T) {
		t.Parallel()
		e := queueEnv(t)
		if code, _, _ := e.run("up"); code != exitNoCapacity || e.clock.slept != 0 {
			t.Errorf("exit %d, slept %d", code, e.clock.slept)
		}
	})
}
