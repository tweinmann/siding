//go:build docker

package e2e

import (
	"context"
	"strings"
	"testing"
	"time"
)

// roomCompose needs 200 MiB by its limit, so with a budget of 200 MiB a second one fits only
// when the first, which uses some memory, is stopped; its volume keeps a file.
const roomCompose = `services:
  web:
    image: busybox:1.37.0
    command: ["sh", "-c", "mkdir -p /www && echo web > /www/index.html && exec httpd -f -p 80 -h /www"]
    expose: ["80"]
    labels: {dev.siding.route: "/"}
    mem_limit: 200m
    volumes: [data:/data]
volumes:
  data: {}
`

func TestMakingRoom(t *testing.T) {
	m := newMachine(t)
	// A budget for one stack of 200 MiB; a stack idle for 10 seconds makes room.
	if code, out := m.siding(t.TempDir(), "setup", "--idle", "10s", "--budget", "200m", "--json"); code != 0 || !strings.Contains(out, `"idle": "10s"`) {
		t.Fatalf("setup: exit %d\n%s", code, out)
	}

	a := repo(t, "rooma", roomCompose, "", "feature/r")
	const projectA, hostA = "siding-test-rooma-feature-r", "siding-test-rooma.feature-r.localhost"
	if code, out := m.siding(a, "up", "--repo", "siding-test-rooma", "--wait", "--json"); code != 0 {
		t.Fatalf("up a: exit %d\n%s", code, out)
	}
	composeExec(t, projectA, "web", "sh", "-c", "echo kept > /data/hello")

	t.Run("the proxy records only router and time", func(t *testing.T) {
		if code, _, _ := get(t, hostA, "/account?token=not-a-real-token"); code != 404 {
			t.Errorf("GET: %d, want the app's 404", code)
		}
		var logs string
		for i := 0; i < 20 && !strings.Contains(logs, projectA); i++ {
			time.Sleep(250 * time.Millisecond)
			logs = run(t, "", "docker", "logs", testProxy.Container)
		}
		if !strings.Contains(logs, `"RouterName":"`+projectA+`-web-route@docker"`) {
			t.Errorf("no line for the request:\n%s", logs)
		}
		if strings.Contains(logs, "not-a-real-token") || strings.Contains(logs, "/account") {
			t.Errorf("the record holds the URL:\n%s", logs)
		}
	})

	// No request for longer than the threshold. Not even siding status: its probes are requests.
	time.Sleep(15 * time.Second)

	b := repo(t, "roomb", roomCompose, "", "feature/r")
	t.Run("up stops the idle stack to make room", func(t *testing.T) {
		code, out, stderr := m.sidingFull(context.Background(), b, "up", "--repo", "siding-test-roomb", "--wait", "--json")
		if code != 0 || decode(t, out).Verdict != "Ready" {
			t.Fatalf("up b: exit %d\n%s", code, out)
		}
		if !strings.Contains(stderr, "Stopped idle stack "+projectA) {
			t.Errorf("stderr does not name the stopped stack:\n%s", stderr)
		}
	})

	t.Run("the stopped stack", func(t *testing.T) {
		code, out := m.siding(a, "status", "--json")
		if code != 2 || decode(t, out).Verdict != "Stopped" || !strings.Contains(out, `"url": "http://`+hostA+`:18080/"`) {
			t.Errorf("status: exit %d\n%s", code, out)
		}
		if code, _, _ := get(t, hostA, "/"); code != 404 {
			t.Errorf("a request to the stopped stack: %d, want 404", code)
		}
	})

	t.Run("up starts it again with its data", func(t *testing.T) {
		if code, out := m.siding(b, "down", "--json"); code != 0 {
			t.Fatalf("down b: exit %d\n%s", code, out)
		}
		if code, out := m.siding(a, "up", "--repo", "siding-test-rooma", "--wait", "--json"); code != 0 || decode(t, out).Verdict != "Ready" {
			t.Fatalf("up a again: exit %d\n%s", code, out)
		}
		if got := composeExec(t, projectA, "web", "cat", "/data/hello"); got != "kept" {
			t.Errorf("data %q, want kept", got)
		}
	})
}
