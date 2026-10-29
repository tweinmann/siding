package cli

import (
	"slices"
	"strings"
	"testing"

	"github.com/tweinmann/siding/internal/docker"
	"github.com/tweinmann/siding/internal/fail"
	"github.com/tweinmann/siding/internal/snapshot"
)

// seedConfig is testConfig with seed labels on db (a seed command) and on a cache service
// (never seeded); web and api use the volume data.
const seedConfig = `{
  "name": "shop-main",
  "services": {
    "db": {"labels": {"dev.siding.seed": "script", "dev.siding.seed.command": "psql -f /seed.sql"}, "networks": {"default": null},
           "volumes": [{"type": "volume", "source": "pgdata", "target": "/var/lib/postgresql"}]},
    "cache": {"labels": {"dev.siding.seed": "none"}, "networks": {"default": null},
              "volumes": [{"type": "volume", "source": "cache", "target": "/cache"}]},
    "web": {"expose": ["80"], "labels": {"dev.siding.route": "/"}, "networks": {"default": null},
            "volumes": [{"type": "volume", "source": "data", "target": "/data"}]}
  },
  "networks": {"default": {"name": "shop-main_default"}},
  "volumes": {"data": {"name": "shop-main_data"}, "pgdata": {"name": "shop-main_pgdata"}, "cache": {"name": "shop-main_cache"}}
}`

// seedEnv is snapshotEnv with seedConfig, and Compose's up starting every service.
func seedEnv(t *testing.T) *env {
	t.Helper()
	e := snapshotEnv(t)
	e.proxyUp(80)
	e.compose.config = seedConfig
	e.compose.onRun = func(r composeRun) {
		if r.args[0] == "up" {
			e.stackUp(e.id(), "cache", "db", "web")
		}
	}
	return e
}

func TestSeedPlanAndFirstUp(t *testing.T) {
	t.Parallel()

	t.Run("invalid labels", func(t *testing.T) {
		t.Parallel()
		e := seedEnv(t)
		e.compose.config = `{"services": {"db": {"labels": {"dev.siding.seed": "script"}}}}`
		if code, stdout, _ := e.run("up", "--json"); code != exitFailed || !strings.Contains(stdout, `"code": "InvalidArguments"`) || len(e.compose.recorded()) != 0 {
			t.Errorf("exit %d, %s", code, stdout)
		}
	})

	t.Run("a later up seeds nothing", func(t *testing.T) {
		t.Parallel()
		e := seedEnv(t)
		e.stackUp(e.id(), "web") // the stack ran before
		code, _, stderr := e.run("up", "--seed", "base")
		if code != exitOK || !strings.Contains(stderr, "already has data, so --seed base seeds nothing") {
			t.Errorf("exit %d, stderr %q", code, stderr)
		}
		if len(e.docker.calls) != 0 || slices.ContainsFunc(composeCalls(e), func(c string) bool { return strings.Contains(c, " exec ") }) {
			t.Errorf("seeded on a later up: docker %q, compose %q", e.docker.calls, composeCalls(e))
		}
	})

	t.Run("unknown snapshot", func(t *testing.T) {
		t.Parallel()
		e := seedEnv(t)
		code, stdout, _ := e.run("up", "--seed", "nope", "--json")
		if code != exitFailed || !strings.Contains(stdout, `"code": "`+fail.SnapshotNotFound+`"`) || !strings.Contains(stdout, "big") || len(e.compose.recorded()) != 0 {
			t.Errorf("exit %d, %s", code, stdout)
		}
	})
}

func TestSeedFromSnapshot(t *testing.T) {
	t.Parallel()
	e := seedEnv(t)
	code, _, stderr := e.run("up", "--seed", "base")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if !strings.Contains(stderr, "Seeding shop-main from the snapshot base: data\n") {
		t.Errorf("up does not say what it seeds:\n%s", stderr)
	}
	// Only data: base has no pgdata (db seeds with its command anyway) and cache says none.
	if want := []string{"create shop-main_data", "copy siding-snap_shop_base_data shop-main_data"}; !slices.Equal(e.docker.calls, want) {
		t.Errorf("docker %q, want %q", e.docker.calls, want)
	}
	var created map[string]string
	for _, v := range e.docker.volumes {
		if v.Name == "shop-main_data" {
			created = v.Labels
		}
	}
	if created["com.docker.compose.project"] != "shop-main" || created["com.docker.compose.volume"] != "data" || created["dev.siding.repo"] != "shop" {
		t.Errorf("labels of the seeded volume: %v", created)
	}
	calls := composeCalls(e)
	if len(calls) != 2 || !strings.HasPrefix(calls[0], "shop-main up -d") || calls[1] != "shop-main exec -T db sh -c psql -f /seed.sql" {
		t.Errorf("compose %q: want up, then the seed command", calls)
	}
}

func TestSeedCommandFails(t *testing.T) {
	t.Parallel()
	for _, wait := range []bool{false, true} {
		e := seedEnv(t)
		e.compose.execErr = fail.New(fail.ComposeFailed, "docker compose exec -T db sh -c psql -f /seed.sql failed with exit code 2")
		e.compose.execOutput = "psql: ERROR: relation \"users\" already exists\npassword not-a-real-token\n"
		args := []string{"up", "--json"}
		if wait {
			args = append(args, "--wait")
		}
		code, stdout, _ := e.run(args...)
		if code != exitFailed || verdictOf(t, stdout) != "Failed" || !strings.Contains(stdout, `"reason": "SeedFailed"`) ||
			!strings.Contains(stdout, "relation") || strings.Contains(stdout, "not-a-real-token") || !strings.Contains(stdout, "exit code 2") {
			t.Errorf("wait %v: exit %d\n%s", wait, code, stdout)
		}
	}
}

// withDefault adds the snapshot default of shop, holding data.
func withDefault(e *env) {
	e.docker.volumes = append(e.docker.volumes, docker.Volume{Name: snapshot.VolumeName("shop", "default", "data"),
		Labels: snapshot.Labels("shop", "default", "data", e.id().Machine, e.clock.now)})
}

func TestDefaultSnapshot(t *testing.T) {
	t.Parallel()

	t.Run("a new worktree starts with data", func(t *testing.T) {
		t.Parallel()
		e := seedEnv(t)
		withDefault(e)
		code, _, stderr := e.run("up")
		if code != exitOK || !strings.Contains(stderr, "The repository's snapshot default seeds this first up; --seed none starts empty") ||
			!strings.Contains(stderr, "Seeding shop-main from the snapshot default: data") {
			t.Fatalf("exit %d, stderr %q", code, stderr)
		}
		if want := []string{"create shop-main_data", "copy siding-snap_shop_default_data shop-main_data"}; !slices.Equal(e.docker.calls, want) {
			t.Errorf("docker %q, want %q", e.docker.calls, want)
		}
	})

	t.Run("empty on purpose", func(t *testing.T) {
		t.Parallel()
		e := seedEnv(t)
		withDefault(e)
		code, _, stderr := e.run("up", "--seed", "none")
		if code != exitOK || len(e.docker.calls) != 0 || strings.Contains(stderr, "snapshot default") {
			t.Errorf("exit %d, docker %q, stderr %q", code, e.docker.calls, stderr)
		}
		// The seed command still runs on a first up: none only means no snapshot.
		if calls := composeCalls(e); len(calls) != 2 || calls[1] != "shop-main exec -T db sh -c psql -f /seed.sql" {
			t.Errorf("compose %q", calls)
		}
	})

	t.Run("no default", func(t *testing.T) {
		t.Parallel()
		e := seedEnv(t) // base and big, no default
		if code, _, stderr := e.run("up"); code != exitOK || len(e.docker.calls) != 0 || strings.Contains(stderr, "Seeding shop-main from") {
			t.Errorf("exit %d, docker %q, stderr %q", code, e.docker.calls, stderr)
		}
	})

	t.Run("not a first up", func(t *testing.T) {
		t.Parallel()
		e := seedEnv(t)
		withDefault(e)
		e.stackUp(e.id(), "web")
		if code, _, stderr := e.run("up"); code != exitOK || len(e.docker.calls) != 0 || strings.Contains(stderr, "warning") {
			t.Errorf("exit %d, docker %q, stderr %q", code, e.docker.calls, stderr)
		}
	})
}
