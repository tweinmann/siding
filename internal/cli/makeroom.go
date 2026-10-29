package cli

import (
	"context"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/tweinmann/siding/internal/budget"
	"github.com/tweinmann/siding/internal/docker"
	"github.com/tweinmann/siding/internal/idle"
	"github.com/tweinmann/siding/internal/stack"
)

// roomInterval is how often a stack waiting in line reads the proxy's log to make room; the
// log can hold 30 MiB, too much for every check of the line.
const roomInterval = 30 * time.Second

// idleStack is a running stack nobody requested for the idle threshold.
type idleStack struct {
	project string
	since   time.Time
	memory  int64
}

// roomMaker stops idle stacks when a new one does not fit (spec idle-stop, design D5). It says
// once per up when the proxy keeps no record of requests.
type roomMaker struct {
	a       *app
	project string
	stderr  io.Writer
	told    bool
	lastTry time.Time
}

// make stops the idle stacks, longest idle first, that the stack needs to fit, and reports
// whether it stopped any. It stops none when even all of them would not make it fit.
func (r *roomMaker) make(ctx context.Context, check budget.Check) (bool, error) {
	a := r.a
	r.lastTry = a.opts.Clock.Now()
	proxyContainer, err := a.opts.Docker.Container(ctx, a.opts.Proxy.Container)
	if err != nil || proxyContainer == nil {
		return false, err
	}
	threshold, on := idleThreshold(proxyContainer)
	if !on {
		return false, nil
	}
	if !idle.Recording(proxyContainer.Cmd) {
		if !r.told {
			warnf(r.stderr, "the proxy keeps no record of requests yet, so no idle stack can make room; siding setup turns it on")
			r.told = true
		}
		return false, nil
	}
	candidates, err := r.idleStacks(ctx, proxyContainer, threshold)
	if err != nil {
		return false, err
	}
	need := check.Used + check.Estimate - check.Budget
	var freed int64
	n := 0
	for n < len(candidates) && freed < need {
		freed += candidates[n].memory
		n++
	}
	if freed < need {
		return false, nil
	}
	now := a.opts.Clock.Now()
	for _, s := range candidates[:n] {
		if err := a.stopProject(ctx, s.project, r.stderr); err != nil {
			return false, err
		}
		_, _ = fmt.Fprintf(r.stderr, "Stopped idle stack %s (no request for %s, %s) to make room for %s; siding up --wait in its worktree starts it again\n",
			s.project, idleText(now.Sub(s.since)), bytesText(uint64(s.memory)), r.project)
	}
	return n > 0, nil
}

// due reports whether the line may read the proxy's log again.
func (r *roomMaker) due() bool {
	return r.lastTry.IsZero() || !r.a.opts.Clock.Now().Before(r.lastTry.Add(roomInterval))
}

// idleStacks lists the other running stacks whose last request is at least threshold ago,
// longest idle first, with their memory in use.
func (r *roomMaker) idleStacks(ctx context.Context, proxyContainer *docker.Container, threshold time.Duration) ([]idleStack, error) {
	a := r.a
	cs, err := a.opts.Docker.Containers(ctx, map[string]string{stack.LabelRepo: ""})
	if err != nil {
		return nil, err
	}
	byProject := map[string][]docker.Container{}
	for _, c := range cs {
		if p := c.Labels[stack.LabelProject]; p != "" && p != r.project {
			byProject[p] = append(byProject[p], c)
		}
	}
	lines, err := a.opts.Docker.Stdout(ctx, proxyContainer.ID)
	if err != nil {
		return nil, err
	}
	last := idle.LastRequests(lines, slices.Collect(maps.Keys(byProject)))
	now := a.opts.Clock.Now()
	var out []idleStack
	for project, containers := range byProject {
		var started time.Time
		running := false
		for _, c := range containers {
			running = running || c.State == "running"
			if c.StartedAt.After(started) {
				started = c.StartedAt
			}
		}
		since := idle.Since(last[project], started, proxyContainer.StartedAt)
		if !running || now.Sub(since) < threshold {
			continue
		}
		memory, err := a.memory(ctx, containers)
		if err != nil {
			return nil, err
		}
		s := idleStack{project: project, since: since}
		for _, m := range memory {
			s.memory += int64(m)
		}
		out = append(out, s)
	}
	slices.SortFunc(out, func(x, y idleStack) int {
		if c := x.since.Compare(y.since); c != 0 {
			return c
		}
		return strings.Compare(x.project, y.project)
	})
	return out, nil
}

// idleText is a duration without a request: 45s, 20m or 2h5m.
func idleText(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d/time.Second))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d/time.Minute))
	}
	return fmt.Sprintf("%dh%dm", int(d/time.Hour), int(d%time.Hour/time.Minute))
}
