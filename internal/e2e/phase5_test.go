//go:build docker

package e2e

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tweinmann/siding/internal/compose"
)

// Compose fails its config on a missing env_file, but lists it without env resolution.
func TestEnvFilesFromCompose(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "compose.yaml")
	content := "services:\n  web:\n    image: busybox:1.37.0\n    env_file: [app.env, {path: opt.env, required: false}]\n"
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	c := &compose.CLI{}
	inv := compose.Invocation{Dir: dir, Project: "siding-test-envfiles", Files: []string{file}}
	if _, err := c.Config(context.Background(), inv); err == nil {
		t.Error("docker compose config accepted a missing env_file")
	}
	data, err := c.UnresolvedConfig(context.Background(), inv)
	if err != nil {
		t.Fatal(err)
	}
	got, err := compose.EnvFiles(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != (compose.EnvFile{Path: filepath.Join(dir, "app.env"), Required: true}) ||
		got[1] != (compose.EnvFile{Path: filepath.Join(dir, "opt.env"), Required: false}) {
		t.Errorf("env files %+v", got)
	}
}

// onbCompose: web reads GREETING from .env and keeps a file in its volume.
const onbCompose = `services:
  web:
    image: busybox:1.37.0
    command: ["sh", "-c", "mkdir -p /www && echo $$GREETING > /www/index.html && exec httpd -f -p 80 -h /www"]
    environment: {GREETING: "${GREETING}"}
    expose: ["80"]
    labels: {dev.siding.route: "/"}
    volumes: [data:/data]
volumes:
  data: {}
`

// onboarding is the user journey of phase 5: a main worktree with an untracked .env, linked
// worktrees without it, and the snapshot default.
func TestOnboarding(t *testing.T) {
	m := newMachine(t)
	if code, out := m.siding(t.TempDir(), "setup", "--json"); code != 0 {
		t.Fatalf("setup: exit %d\n%s", code, out)
	}
	base := t.TempDir()
	main := filepath.Join(base, "onb")
	if err := os.MkdirAll(main, 0o755); err != nil {
		t.Fatal(err)
	}
	gitIn(t, main, "init", "-q", "-b", "main")
	for file, content := range map[string]string{"compose.yaml": onbCompose, ".gitignore": ".env\n"} {
		if err := os.WriteFile(filepath.Join(main, file), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gitIn(t, main, "add", ".")
	gitIn(t, main, "commit", "-q", "-m", "app")
	if err := os.WriteFile(filepath.Join(main, ".env"), []byte("GREETING=hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	const repoFlag = "siding-test-onb"
	for _, p := range []string{"siding-test-onb-main", "siding-test-onb-feature-b", "siding-test-onb-feature-c"} {
		t.Cleanup(func() { run(t, "", "docker", "compose", "-p", p, "down", "-v", "--remove-orphans") })
	}
	t.Cleanup(func() { run(t, "", "docker", "volume", "rm", "siding-snap_siding-test-onb_default_data") })

	if code, out := m.siding(main, "up", "--repo", repoFlag, "--wait", "--json"); code != 0 {
		t.Fatalf("up main: exit %d\n%s", code, out)
	}
	composeExec(t, "siding-test-onb-main", "web", "sh", "-c", "echo from-main > /data/hello")
	if code, out := m.siding(main, "snapshot", "save", "default", "--json"); code != 0 {
		t.Fatalf("save default: exit %d\n%s", code, out)
	}

	b := filepath.Join(base, "onb-feature-b")
	gitIn(t, main, "worktree", "add", "-q", "-b", "feature/b", b)
	t.Run("a new worktree without .env is refused", func(t *testing.T) {
		code, out := m.siding(b, "up", "--repo", repoFlag, "--json")
		s := decode(t, out)
		if code != 1 || s.Code != "EnvFileMissing" || !strings.Contains(out, "cp "+filepath.Join(main, ".env")+" "+filepath.Join(b, ".env")) {
			t.Fatalf("exit %d\n%s", code, out)
		}
		if out := run(t, "", "docker", "ps", "-aq", "--filter", "label=com.docker.compose.project=siding-test-onb-feature-b"); strings.TrimSpace(out) != "" {
			t.Errorf("containers started: %q", out)
		}
		if code, out := m.siding(b, "lint", "--json"); !strings.Contains(out, `"rule": "MissingEnvFile"`) {
			t.Errorf("lint: exit %d\n%s", code, out)
		}
	})

	t.Run("after the cp, it starts from the snapshot default", func(t *testing.T) {
		data, err := os.ReadFile(filepath.Join(main, ".env"))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(b, ".env"), data, 0o644); err != nil {
			t.Fatal(err)
		}
		code, out, stderr := m.sidingFull(context.Background(), b, "up", "--repo", repoFlag, "--wait", "--json")
		if code != 0 || decode(t, out).Verdict != "Ready" || !strings.Contains(stderr, "snapshot default seeds this first up") {
			t.Fatalf("exit %d, stderr %q\n%s", code, stderr, out)
		}
		if got := composeExec(t, "siding-test-onb-feature-b", "web", "cat", "/data/hello"); got != "from-main" {
			t.Errorf("seeded data %q", got)
		}
		if _, body, _ := get(t, "siding-test-onb.feature-b.localhost", "/"); body != "hello" {
			t.Errorf("GREETING from .env: %q", body)
		}
	})

	t.Run("--seed none starts empty", func(t *testing.T) {
		c := filepath.Join(base, "onb-feature-c")
		gitIn(t, main, "worktree", "add", "-q", "-b", "feature/c", c)
		if err := os.WriteFile(filepath.Join(c, ".env"), []byte("GREETING=hi\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if code, out := m.siding(c, "up", "--repo", repoFlag, "--seed", "none", "--wait", "--json"); code != 0 {
			t.Fatalf("up: exit %d\n%s", code, out)
		}
		if got := composeExec(t, "siding-test-onb-feature-c", "web", "sh", "-c", "cat /data/hello 2>/dev/null || echo empty"); got != "empty" {
			t.Errorf("data %q, want empty", got)
		}
	})

	t.Run("skill --install", func(t *testing.T) {
		if code, out := m.siding(main, "skill", "--install"); code != 0 || !strings.Contains(out, "Commit it") {
			t.Fatalf("exit %d\n%s", code, out)
		}
		if _, err := os.Stat(filepath.Join(main, ".claude", "skills", "siding", "SKILL.md")); err != nil {
			t.Error(err)
		}
	})
}
