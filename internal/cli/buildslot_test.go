package cli

import (
	"strings"
	"testing"

	"github.com/tweinmann/siding/internal/buildslot"
)

// buildConfig is a project with one service built from a Dockerfile.
const buildConfig = `{"name": "shop-main", "services": {"app": {"build": {"context": "/src/shop/app"}, "networks": {"default": null}}},
  "networks": {"default": {"name": "shop-main_default"}}}`

func TestBuildSlots(t *testing.T) {
	t.Parallel()

	// All slots are held by other ups; the 1st sleep frees one.
	holdAll := func(t *testing.T, e *env) []*buildslot.Slot {
		setupDoc(t, e, "--builds", "2")
		var held []*buildslot.Slot
		for range 2 {
			s, ok, err := buildslot.TryAcquire(e.opts.Home, 2)
			if err != nil || !ok {
				t.Fatalf("holding a slot: %v %v", ok, err)
			}
			held = append(held, s)
		}
		return held
	}

	t.Run("waits for a slot", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t)
		e.compose.config = buildConfig
		held := holdAll(t, e)
		e.clock.onSleep = func(n int) {
			if n == 1 {
				held[0].Release()
			}
		}
		code, _, stderr := e.run("up")
		// At least one round: a git child of a parallel test may hold the freed lock for a moment
		// between its fork and exec, and up simply tries again.
		if code != exitOK || !strings.Contains(stderr, "Waiting for a build slot: 2 stacks build at once") || e.clock.slept < 1 {
			t.Errorf("exit %d, slept %d, stderr %q", code, e.clock.slept, stderr)
		}
		held[1].Release()
	})

	t.Run("time is up", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t)
		e.compose.config = buildConfig
		held := holdAll(t, e)
		code, stdout, _ := e.run("up", "--timeout", "5s", "--json")
		if code != exitNoCapacity || !strings.Contains(stdout, "build slot") || len(e.compose.recorded()) != 1 { // only setup's
			t.Errorf("exit %d, %s", code, stdout)
		}
		for _, s := range held {
			s.Release()
		}
	})

	t.Run("images only need no slot", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t)
		held := holdAll(t, e)
		if code, _, stderr := e.run("up"); code != exitOK || strings.Contains(stderr, "build slot") || e.clock.slept != 0 {
			t.Errorf("exit %d, stderr %q", code, stderr)
		}
		for _, s := range held {
			s.Release()
		}
	})
}
