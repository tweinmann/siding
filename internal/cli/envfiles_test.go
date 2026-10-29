package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tweinmann/siding/internal/fail"
	"github.com/tweinmann/siding/internal/testutil"
)

// linkedEnv: the main worktree /src/shop with an untracked .env, and a linked worktree
// /src/shop-feature-x without it, where siding runs.
func linkedEnv(t *testing.T) (e *env, linked string) {
	t.Helper()
	e = newEnv(t)
	e.git("rm", "-q", "--cached", ".env")
	e.git("commit", "-q", "-m", "no .env in git")
	linked = filepath.Join(e.base, "shop-feature-x")
	e.git("worktree", "add", "-q", "-b", "feature/x", linked)
	e.opts.Dir = linked
	e.proxyUp(80)
	// What Compose prints for this worktree's project.
	e.compose.config = strings.ReplaceAll(testConfig, "shop-main", "shop-feature-x")
	return e, linked
}

func composeUps(e *env) int {
	n := 0
	for _, r := range e.compose.recorded() {
		if r.args[0] == "up" {
			n++
		}
	}
	return n
}

func TestEnvFiles(t *testing.T) {
	t.Parallel()

	t.Run("a new worktree without .env", func(t *testing.T) {
		t.Parallel()
		e, _ := linkedEnv(t)
		code, stdout, _ := e.run("up", "--json")
		if code != exitFailed || composeUps(e) != 0 {
			t.Fatalf("exit %d, compose ups %d\n%s", code, composeUps(e), stdout)
		}
		testutil.Golden(t, filepath.Join("testdata", "json", "error-envfilemissing.json"), []byte(stdout))
		if !strings.Contains(stdout, "cp /src/shop/.env /src/shop-feature-x/.env") || strings.Contains(stdout, "not-a-real-token") {
			t.Errorf("hint or a value:\n%s", stdout)
		}
	})

	t.Run("an env_file", func(t *testing.T) {
		t.Parallel()
		e, linked := linkedEnv(t)
		if err := os.WriteFile(filepath.Join(linked, ".env"), nil, 0o644); err != nil {
			t.Fatal(err)
		}
		e.write("api.env", "API_TOKEN=not-a-real-api-token\n")
		e.compose.unresolved = `{"services": {"api": {"env_file": [{"path": "` + filepath.Join(linked, "api.env") + `"},
			{"path": "` + filepath.Join(linked, "optional.env") + `", "required": false}]}}}`
		code, stdout, _ := e.run("up", "--json")
		if code != exitFailed || !strings.Contains(stdout, `"code": "`+fail.EnvFileMissing+`"`) ||
			!strings.Contains(stdout, "lacks api.env,") || strings.Contains(stdout, "optional.env") {
			t.Errorf("exit %d\n%s", code, stdout)
		}
	})

	t.Run("after the cp of the hint", func(t *testing.T) {
		t.Parallel()
		e, linked := linkedEnv(t)
		data, err := os.ReadFile(filepath.Join(e.repo, ".env"))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(linked, ".env"), data, 0o644); err != nil {
			t.Fatal(err)
		}
		if code, _, stderr := e.run("up"); code != exitOK || composeUps(e) != 1 {
			t.Errorf("exit %d, stderr %q", code, stderr)
		}
	})

	t.Run("not needed", func(t *testing.T) {
		t.Parallel()
		e, _ := linkedEnv(t)
		if err := os.Remove(filepath.Join(e.repo, ".env")); err != nil {
			t.Fatal(err)
		}
		if code, _, stderr := e.run("up"); code != exitOK || composeUps(e) != 1 {
			t.Errorf("main has no .env: exit %d, stderr %q", code, stderr)
		}
	})

	t.Run("tracked", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t) // .env is committed: every worktree has it
		linked := filepath.Join(e.base, "shop-feature-x")
		e.git("worktree", "add", "-q", "-b", "feature/x", linked)
		e.opts.Dir = linked
		e.proxyUp(80)
		if err := os.Remove(filepath.Join(linked, ".env")); err != nil {
			t.Fatal(err)
		}
		if code, _, stderr := e.run("up"); code != exitOK {
			t.Errorf("a tracked .env deleted on the branch: exit %d, stderr %q", code, stderr)
		}
	})

	t.Run("the main worktree", func(t *testing.T) {
		t.Parallel()
		e, _ := linkedEnv(t)
		e.opts.Dir = e.repo
		if err := os.Remove(filepath.Join(e.repo, ".env")); err != nil {
			t.Fatal(err)
		}
		if code, _, stderr := e.run("up"); code != exitOK {
			t.Errorf("exit %d, stderr %q", code, stderr)
		}
	})
}

func TestLintEnvFiles(t *testing.T) {
	t.Parallel()

	t.Run(".env missing", func(t *testing.T) {
		t.Parallel()
		e, _ := linkedEnv(t)
		code, stdout, _ := e.run("lint")
		if code != exitOK || !strings.Contains(stdout, "warning MissingEnvFile, file .env:") ||
			!strings.Contains(stdout, "cp /src/shop/.env /src/shop-feature-x/.env") {
			t.Errorf("exit %d\n%s", code, stdout)
		}
	})

	t.Run("an env_file missing", func(t *testing.T) {
		t.Parallel()
		e, linked := linkedEnv(t)
		if err := os.WriteFile(filepath.Join(linked, ".env"), nil, 0o644); err != nil {
			t.Fatal(err)
		}
		e.write("api.env", "API_TOKEN=not-a-real-api-token\n")
		e.compose.unresolved = strings.Replace(e.compose.config, `"api": {`, `"api": {"env_file": [{"path": "`+filepath.Join(linked, "api.env")+`"}], `, 1)
		e.compose.configErr = fail.New(fail.ComposeFailed, "env file %s not found", filepath.Join(linked, "api.env"))
		code, stdout, _ := e.run("lint", "--json")
		// The other rules still run, on the configuration without env files.
		if code != exitOK || !strings.Contains(stdout, `"rule": "MissingEnvFile"`) || !strings.Contains(stdout, `"subject": "file api.env"`) ||
			!strings.Contains(stdout, `"rule": "PublishedPort"`) {
			t.Errorf("exit %d\n%s", code, stdout)
		}
	})
}
