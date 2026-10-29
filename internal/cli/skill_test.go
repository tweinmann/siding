package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tweinmann/siding/internal/fail"
	"github.com/tweinmann/siding/internal/testutil"
)

func TestSkill(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	code, text, _ := e.run("skill")
	if code != exitOK {
		t.Fatalf("exit %d", code)
	}
	testutil.Golden(t, filepath.Join("testdata", "skill", "SKILL.md"), []byte(text))

	if !strings.HasPrefix(text, "---\nname: siding\ndescription: ") {
		t.Errorf("not a skill named siding:\n%s", text[:min(len(text), 80)])
	}
	// The same file works in any harness that reads the Agent Skills format.
	if strings.Contains(text, "Claude Code") {
		t.Error("the skill names a harness")
	}
	for _, want := range []string{"siding up --wait --json", "siding status --json", "siding logs --json", "**0, `Ready`**", "**1, `Failed`**", "**2, `TimedOut`**", "**2, `Stopped`**", "**3, `NoCapacity`**",
		"`stack.url`", "Never give\n  `localhost:<port>`"} {
		if !strings.Contains(text, want) {
			t.Errorf("the skill does not mention %q", want)
		}
	}
	// Only a human removes data, forces anything, or exposes a stack.
	// The idle threshold is the user's decision too.
	for _, never := range []string{"--force", "--volumes", "--idle", "share", "gc"} {
		if strings.Contains(text, never) {
			t.Errorf("the skill mentions %q", never)
		}
	}

	code, stdout, _ := e.run("skill", "--json")
	if code != exitOK {
		t.Fatalf("exit %d", code)
	}
	testutil.Golden(t, filepath.Join("testdata", "json", "skill.json"), []byte(stdout))
	var doc skillDocument
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil || doc.Text != text || doc.Name != "siding" {
		t.Errorf("document %+v, err %v", doc, err)
	}
}

func TestSkillInstall(t *testing.T) {
	t.Parallel()

	t.Run("into the repository, from a subdirectory", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t)
		e.write(".claude/settings.json", `{"permissions": {"allow": []}}`+"\n")
		e.write("web/app.js", "")
		e.opts.Dir = filepath.Join(e.repo, "web")
		code, stdout, stderr := e.run("skill", "--install")
		if code != exitOK || stderr != "" {
			t.Fatalf("exit %d, stderr %q", code, stderr)
		}
		data, err := os.ReadFile(filepath.Join(e.repo, ".claude", "skills", "siding", "SKILL.md"))
		if err != nil || string(data) != skillText {
			t.Fatalf("skill not written: %v", err)
		}
		for _, want := range []string{"Wrote /src/shop/.claude/skills/siding/SKILL.md", "Commit it, so every worktree has it",
			"## Running the app with siding", `"Bash(siding up:*)"`, `"Bash(siding status:*)"`, `"Bash(siding logs:*)"`,
			`"Bash(siding ps:*)"`, "allow siding up, siding status, siding logs, siding ps"} {
			if !strings.Contains(stdout, want) {
				t.Errorf("output lacks %q:\n%s", want, stdout)
			}
		}
		// Only reading commands: what removes or stops anything stays the human's.
		for _, never := range []string{"down", "gc", "snapshot rm", "teardown", "--force", "--volumes"} {
			if strings.Contains(stdout, never) {
				t.Errorf("output mentions %q", never)
			}
		}
		if got, _ := os.ReadFile(filepath.Join(e.repo, ".claude", "settings.json")); string(got) != `{"permissions": {"allow": []}}`+"\n" {
			t.Errorf("settings.json changed: %s", got)
		}
		if code, stdout, _ := e.run("skill", "--install"); code != exitOK || !strings.HasPrefix(stdout, "Updated ") {
			t.Errorf("again: exit %d, %q", code, stdout)
		}
	})

	t.Run("another harness", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t)
		code, stdout, _ := e.run("skill", "--install", "--dir", "~/.agents/skills")
		want := filepath.Join(e.opts.Home, ".agents", "skills", "siding", "SKILL.md")
		if _, err := os.Stat(want); code != exitOK || err != nil {
			t.Fatalf("exit %d, %v\n%s", code, err, stdout)
		}
		if strings.Contains(stdout, "Commit it") {
			t.Error("a skill outside the repository needs no commit")
		}
		if _, err := os.Stat(filepath.Join(e.repo, ".claude")); err == nil {
			t.Error("--dir wrote under .claude")
		}
	})

	t.Run("JSON", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t)
		code, stdout, stderr := e.run("skill", "--install", "--json")
		if code != exitOK || !strings.Contains(stderr, "## Running the app with siding") {
			t.Fatalf("exit %d, stderr %q", code, stderr)
		}
		var doc skillDocument
		if err := json.Unmarshal([]byte(stdout), &doc); err != nil || doc.Path != "/src/shop/.claude/skills/siding/SKILL.md" || doc.Text != skillText {
			t.Errorf("document %+v, err %v", doc.Path, err)
		}
	})

	t.Run("refusals", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t)
		if code, stdout, _ := e.run("skill", "--dir", "x", "--json"); code != exitFailed || !strings.Contains(stdout, `"code": "`+fail.InvalidArguments+`"`) {
			t.Errorf("--dir without --install: exit %d, %s", code, stdout)
		}
		e.opts.Dir = t.TempDir() // not a git worktree
		if code, stdout, _ := e.run("skill", "--install", "--json"); code != exitFailed || !strings.Contains(stdout, `"code": "`+fail.NotAGitRepository+`"`) {
			t.Errorf("outside a worktree: exit %d, %s", code, stdout)
		}
	})
}
