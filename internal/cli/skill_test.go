package cli

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

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
	for _, want := range []string{"siding up --wait --json", "siding status --json", "siding logs --json", "**0, `Ready`**", "**1, `Failed`**", "**2, `TimedOut`**", "**3, `NoCapacity`**"} {
		if !strings.Contains(text, want) {
			t.Errorf("the skill does not mention %q", want)
		}
	}
	// Only a human removes data, forces anything, or exposes a stack.
	for _, never := range []string{"--force", "--volumes", "share", "gc"} {
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
