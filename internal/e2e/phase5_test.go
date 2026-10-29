//go:build docker

package e2e

import (
	"context"
	"os"
	"path/filepath"
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
