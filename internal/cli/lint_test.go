package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tweinmann/siding/internal/testutil"
)

func TestLint(t *testing.T) {
	t.Parallel()
	lintConfig, err := os.ReadFile(filepath.Join("..", "compose", "testdata", "lint", "config.json"))
	if err != nil {
		t.Fatal(err)
	}

	t.Run("errors as JSON", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t)
		e.compose.config = string(lintConfig)
		code, stdout, _ := e.run("lint", "--json")
		if code != exitFailed {
			t.Fatalf("exit %d: %s", code, stdout)
		}
		testutil.Golden(t, filepath.Join("testdata", "json", "lintreport.json"), []byte(stdout))
	})

	t.Run("text", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t)
		e.compose.config = string(lintConfig)
		_, stdout, _ := e.run("lint")
		for _, want := range []string{
			"error   ContainerName, service db: db sets container_name \"shop-db\", so a second stack of this file cannot start\n        hint: remove container_name",
			"info    NoRoot, stack:",
			"/src/shop/compose.yaml: 3 errors, 7 warnings, 3 info\n",
		} {
			if !strings.Contains(stdout, want) {
				t.Errorf("the text lacks %q:\n%s", want, stdout)
			}
		}
	})

	t.Run("clean", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t)
		e.compose.config = `{"name": "shop-main", "services": {"web": {"expose": ["80"], "labels": {"dev.siding.route": "/"},
			"networks": {"default": null}, "healthcheck": {"test": ["CMD", "true"]}}}, "networks": {"default": {"name": "shop-main_default"}}}`
		if code, stdout, _ := e.run("lint"); code != exitOK || stdout != "/src/shop/compose.yaml: no findings\n" {
			t.Errorf("exit %d, stdout %q", code, stdout)
		}
	})

	t.Run("outside a repository", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t)
		dir := filepath.Join(e.base, "plain")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "compose.yml"), []byte("services: {}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		e.opts.Dir = dir
		if code, stdout, stderr := e.run("lint", "--json"); !strings.Contains(stdout, `"file": "/src/plain/compose.yml"`) || code == exitFailed && strings.Contains(stdout, `"kind": "Error"`) {
			t.Errorf("exit %d, stdout %s, stderr %s", code, stdout, stderr)
		}
	})
}
