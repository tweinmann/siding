//go:build docker

package e2e

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// db keeps a file in its volume; worker seeds its own volume with a command.
const snapCompose = `services:
  db:
    image: busybox:1.37.0
    command: ["sh", "-c", "[ -f /data/hello ] || echo first > /data/hello; exec sleep 600"]
    volumes: [data:/data]
    healthcheck: {test: ["CMD", "test", "-f", "/data/hello"], interval: 1s, retries: 30}
  worker:
    image: busybox:1.37.0
    command: ["sleep", "600"]
    volumes: [marks:/marks]
    labels:
      dev.siding.seed: script
      dev.siding.seed.command: "echo seeded > /marks/marker"
volumes:
  data: {}
  marks: {}
`

const failingSeedCompose = `services:
  db:
    image: busybox:1.37.0
    command: ["sleep", "600"]
    labels:
      dev.siding.seed: script
      dev.siding.seed.command: "echo the seed broke; exit 2"
`

// hog holds about 300 MB once it is healthy. Not exec sleep: the shell must stay, it holds x.
const hogCompose = `services:
  hog:
    image: busybox:1.37.0
    command: ["sh", "-c", "x=$$(yes | head -c 300000000); touch /tmp/ready; while true; do sleep 60; done"]
    healthcheck: {test: ["CMD", "test", "-f", "/tmp/ready"], interval: 1s, retries: 60}
`

// small needs 300 MiB by its limits.
const smallCompose = `services:
  a:
    image: busybox:1.37.0
    command: ["sleep", "600"]
    mem_limit: 150m
  b:
    image: busybox:1.37.0
    command: ["sleep", "600"]
    mem_limit: 150m
`

const buildCompose = `services:
  app:
    build: ./app
    command: ["sleep", "600"]
`

func composeExec(t *testing.T, project, service string, cmd ...string) string {
	t.Helper()
	return strings.TrimSpace(run(t, "", "docker", append([]string{"compose", "-p", project, "exec", "-T", service}, cmd...)...))
}

func TestSnapshotsQueueAndBuilds(t *testing.T) {
	m := newMachine(t)
	if code, out := m.siding(t.TempDir(), "setup", "--json"); code != 0 {
		t.Fatalf("setup: exit %d\n%s", code, out)
	}
	removeSnapshot := func(volumes ...string) {
		t.Cleanup(func() {
			for _, v := range volumes {
				run(t, "", "docker", "volume", "rm", v) // by exact name
			}
		})
	}
	removeSnapshot("siding-snap_siding-test-snap_base_data", "siding-snap_siding-test-snap_base_marks")

	t.Run("save, seed, and the snapshot outlives the stack", func(t *testing.T) {
		a := repo(t, "snap", snapCompose, "", "feature/a")
		if code, out := m.siding(a, "up", "--repo", "siding-test-snap", "--wait", "--json"); code != 0 {
			t.Fatalf("up a: exit %d\n%s", code, out)
		}
		if got := composeExec(t, "siding-test-snap-feature-a", "worker", "cat", "/marks/marker"); got != "seeded" {
			t.Errorf("the seed command did not run on the first up: %q", got)
		}
		composeExec(t, "siding-test-snap-feature-a", "db", "sh", "-c", "echo from-a > /data/hello")
		if code, out := m.siding(a, "snapshot", "save", "base", "--json"); code != 0 || !strings.Contains(out, "siding-snap_siding-test-snap_base_data") {
			t.Fatalf("save: exit %d\n%s", code, out)
		}
		if code, out := m.siding(a, "snapshot", "save", "base", "--json"); code != 1 || decode(t, out).Code != "SnapshotExists" {
			t.Errorf("save again: exit %d\n%s", code, out)
		}

		b := repo(t, "snap", snapCompose, "", "feature/b")
		code, out, stderr := m.sidingFull(context.Background(), b, "up", "--repo", "siding-test-snap", "--seed", "base", "--wait", "--json")
		if code != 0 || decode(t, out).Verdict != "Ready" {
			t.Fatalf("up --seed: exit %d\n%s", code, out)
		}
		if strings.Contains(stderr, "not created by Docker Compose") {
			t.Errorf("Compose did not adopt the seeded volume:\n%s", stderr)
		}
		if got := composeExec(t, "siding-test-snap-feature-b", "db", "cat", "/data/hello"); got != "from-a" {
			t.Errorf("seeded data %q, want from-a", got)
		}

		if code, out := m.siding(a, "down", "--volumes", "--force", "--json"); code != 0 {
			t.Fatalf("down a: exit %d\n%s", code, out)
		}
		if code, out := m.siding(b, "snapshot", "ls", "--json"); code != 0 || !strings.Contains(out, `"name": "base"`) {
			t.Errorf("the snapshot did not outlive its stack: exit %d\n%s", code, out)
		}
		if code, out := m.siding(b, "snapshot", "rm", "base", "--json"); code != 0 {
			t.Errorf("rm: exit %d\n%s", code, out)
		}
		if out := run(t, "", "docker", "volume", "ls", "-q", "--filter", "label=dev.siding.snapshot=base"); strings.TrimSpace(out) != "" {
			t.Errorf("snapshot volumes left: %q", out)
		}
	})

	t.Run("a failing seed command", func(t *testing.T) {
		dir := repo(t, "seedfail", failingSeedCompose, "", "feature/f")
		code, out := m.siding(dir, "up", "--repo", "siding-test-seedfail", "--wait", "--json")
		s := decode(t, out)
		if code != 1 || s.Failure.Reason != "SeedFailed" || !strings.Contains(strings.Join(s.Failure.Logs, "\n"), "the seed broke") {
			t.Errorf("exit %d\n%s", code, out)
		}
	})

	t.Run("waiting in line", func(t *testing.T) {
		if code, out := m.siding(t.TempDir(), "setup", "--budget", "500m", "--json"); code != 0 {
			t.Fatalf("setup --budget: exit %d\n%s", code, out)
		}
		t.Cleanup(func() { m.siding(t.TempDir(), "setup", "--budget", "default") })
		hog := repo(t, "hog", hogCompose, "", "feature/h")
		if code, out := m.siding(hog, "up", "--repo", "siding-test-hog", "--force", "--wait", "--json"); code != 0 {
			t.Fatalf("up hog: exit %d\n%s", code, out)
		}
		small := repo(t, "small", smallCompose, "", "feature/s")
		done := make(chan int, 1)
		go func() {
			code, _ := m.siding(small, "up", "--repo", "siding-test-small", "--wait", "--timeout", "90s", "--json")
			done <- code
		}()
		// Wait until small is in line, then free the memory.
		queued := false
		for i := 0; i < 60 && !queued; i++ {
			time.Sleep(500 * time.Millisecond)
			entries, _ := os.ReadDir(filepath.Join(m.home, ".siding", "queue"))
			queued = len(entries) > 0
		}
		if !queued {
			t.Fatal("small never waited in line")
		}
		if code, out := m.siding(t.TempDir(), "ps", "--json"); code != 0 || !strings.Contains(out, `"waiting"`) || !strings.Contains(out, "siding-test-small-feature-s") {
			t.Errorf("ps does not show the line: exit %d\n%s", code, out)
		}
		if code, out := m.siding(hog, "down", "--json"); code != 0 {
			t.Fatalf("down hog: exit %d\n%s", code, out)
		}
		select {
		case code := <-done:
			if code != 0 {
				t.Errorf("small ended with %d, want 0", code)
			}
		case <-time.After(2 * time.Minute):
			t.Fatal("small never started")
		}
	})

	t.Run("one build at a time", func(t *testing.T) {
		if code, out := m.siding(t.TempDir(), "setup", "--builds", "1", "--json"); code != 0 {
			t.Fatalf("setup --builds: exit %d\n%s", code, out)
		}
		t.Cleanup(func() { m.siding(t.TempDir(), "setup", "--builds", "default") })
		var stderrs []string
		var mu sync.Mutex
		var wg sync.WaitGroup
		for _, name := range []string{"builda", "buildb"} {
			project := "siding-test-" + name + "-feature-x"
			// Registered before repo's cleanup, so it runs after it: the image is free once the
			// stack is down.
			t.Cleanup(func() { run(t, "", "docker", "image", "rm", project+"-app") })
			dir := repo(t, name, buildCompose, "", "feature/x")
			if err := os.MkdirAll(filepath.Join(dir, "app"), 0o755); err != nil {
				t.Fatal(err)
			}
			// A build of a few seconds, different per repository so neither is cached.
			dockerfile := "FROM busybox:1.37.0\nRUN echo " + name + " > /built && sleep 4\n"
			if err := os.WriteFile(filepath.Join(dir, "app", "Dockerfile"), []byte(dockerfile), 0o644); err != nil {
				t.Fatal(err)
			}
			wg.Add(1)
			go func(dir, name string) {
				defer wg.Done()
				code, out, stderr := m.sidingFull(context.Background(), dir, "up", "--repo", "siding-test-"+name, "--json")
				if code != 0 {
					t.Errorf("up %s: exit %d\n%s", name, code, out)
				}
				mu.Lock()
				stderrs = append(stderrs, stderr)
				mu.Unlock()
			}(dir, name)
		}
		wg.Wait()
		waited := 0
		for _, s := range stderrs {
			if strings.Contains(s, "Waiting for a build slot") {
				waited++
			}
		}
		if waited != 1 {
			t.Errorf("%d of the two ups waited for the build slot, want 1", waited)
		}
	})
}
