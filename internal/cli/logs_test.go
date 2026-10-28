package cli

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tweinmann/siding/internal/testutil"
)

func TestLogs(t *testing.T) {
	t.Parallel()

	t.Run("json, redacted", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t)
		e.proxyUp(80)
		e.stackUp(e.id(), "api", "db", "mail", "web")
		e.docker.logs["shop-main-api-1"] = []string{"one", "token not-a-real-api-token accepted", "three"}
		e.docker.logs["shop-main-db-1"] = []string{"listening on 5432", "password not-a-real-token"}
		code, stdout, _ := e.run("logs", "--json", "--tail", "2", "api", "db")
		if code != exitOK {
			t.Fatalf("exit %d: %s", code, stdout)
		}
		testutil.Golden(t, filepath.Join("testdata", "json", "logs.json"), []byte(stdout))
		if strings.Contains(stdout, "not-a-real") {
			t.Error("a secret reached the document")
		}
	})

	t.Run("text goes through compose", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t)
		e.stackUp(e.id(), "api")
		if code, _, stderr := e.run("logs", "-f", "--tail", "5", "api"); code != exitOK {
			t.Fatalf("exit %d: %s", code, stderr)
		}
		runs := e.compose.recorded()
		if len(runs) != 1 || runs[0].inv.Project != "shop-main" || len(runs[0].inv.Files) != 0 ||
			!slices.Equal(runs[0].args, []string{"logs", "--follow", "--tail", "5", "api"}) {
			t.Errorf("compose runs %+v", runs)
		}
	})

	t.Run("json and follow", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t)
		if code, stdout, _ := e.run("logs", "--json", "-f"); code != exitFailed || !strings.Contains(stdout, `"code": "InvalidArguments"`) {
			t.Errorf("exit %d, stdout %s", code, stdout)
		}
	})

	t.Run("unknown service", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t)
		e.stackUp(e.id(), "api")
		if code, stdout, _ := e.run("logs", "--json", "nope"); code != exitFailed || !strings.Contains(stdout, `"code": "InvalidArguments"`) {
			t.Errorf("exit %d, stdout %s", code, stdout)
		}
	})
}
