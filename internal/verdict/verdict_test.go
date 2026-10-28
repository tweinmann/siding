package verdict

import (
	"testing"
	"time"

	"github.com/tweinmann/siding/internal/docker"
	"github.com/tweinmann/siding/internal/stack"
)

func ct(service, state, health string, exit int) docker.Container {
	return docker.Container{ID: service, Labels: map[string]string{stack.LabelService: service}, State: state, Health: health, ExitCode: exit}
}

func TestEvaluate(t *testing.T) {
	t.Parallel()
	routes := map[string]Route{"web": {URL: "http://shop.main.localhost/", Port: 3000}}
	tests := []struct {
		name       string
		services   []string
		containers []docker.Container
		answering  map[string]bool
		base       map[string]int
		final      bool
		verdict    string
		service    string
		reason     string
	}{
		{
			name:       "healthy, answering and done",
			services:   []string{"db", "migrate", "web"},
			containers: []docker.Container{ct("db", "running", "healthy", 0), ct("migrate", "exited", "", 0), ct("web", "running", "", 0)},
			answering:  map[string]bool{"web": true},
			verdict:    Ready,
		},
		{
			name:       "running but not answering",
			services:   []string{"web"},
			containers: []docker.Container{ct("web", "running", "", 0)},
			verdict:    Starting,
		},
		{
			name:       "healthcheck starting",
			services:   []string{"db"},
			containers: []docker.Container{ct("db", "running", "starting", 0)},
			verdict:    Starting,
		},
		{
			name:       "exited non-zero",
			services:   []string{"api", "db"},
			containers: []docker.Container{ct("api", "exited", "", 1), ct("db", "running", "starting", 0)},
			verdict:    Failed, service: "api", reason: Exited,
		},
		{
			name:       "unhealthy",
			services:   []string{"db"},
			containers: []docker.Container{{ID: "db", Labels: map[string]string{stack.LabelService: "db"}, State: "running", Health: "unhealthy", HealthOutput: "pg_isready: no response"}},
			verdict:    Failed, service: "db", reason: Unhealthy,
		},
		{
			name:       "missing",
			services:   []string{"db", "web"},
			containers: []docker.Container{ct("web", "running", "", 0)},
			answering:  map[string]bool{"web": true},
			verdict:    Failed, service: "db", reason: Missing,
		},
		{
			name:       "restart loop",
			services:   []string{"api"},
			containers: []docker.Container{{ID: "api", Labels: map[string]string{stack.LabelService: "api"}, State: "running", RestartCount: 5}},
			base:       map[string]int{"api": 2},
			verdict:    Failed, service: "api", reason: Restarting,
		},
		{
			name:       "restarts from before the wait do not count",
			services:   []string{"api"},
			containers: []docker.Container{{ID: "api", Labels: map[string]string{stack.LabelService: "api"}, State: "running", RestartCount: 5}},
			base:       map[string]int{"api": 5},
			verdict:    Ready,
		},
		{
			name:       "timed out not answering",
			services:   []string{"web"},
			containers: []docker.Container{ct("web", "running", "", 0)},
			final:      true,
			verdict:    TimedOut, service: "web", reason: NotAnswering,
		},
		{
			name:       "timed out starting",
			services:   []string{"db", "web"},
			containers: []docker.Container{ct("db", "running", "starting", 0), ct("web", "running", "", 0)},
			final:      true,
			verdict:    TimedOut, service: "db", reason: NotReady,
		},
		{
			name:       "the first failure in compose order is reported",
			services:   []string{"a", "b"},
			containers: []docker.Container{ct("b", "exited", "", 2), ct("a", "exited", "", 3)},
			verdict:    Failed, service: "a", reason: Exited,
		},
		{
			name:     "a scaled service: one bad replica fails it",
			services: []string{"worker"},
			containers: []docker.Container{
				{ID: "w1", Labels: map[string]string{stack.LabelService: "worker"}, State: "running"},
				{ID: "w2", Labels: map[string]string{stack.LabelService: "worker"}, State: "exited", ExitCode: 137},
			},
			verdict: Failed, service: "worker", reason: Exited,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			res := Evaluate(Input{Services: tt.services, Containers: tt.containers, Routes: routes,
				Answering: tt.answering, RestartBase: tt.base, Final: tt.final, Timeout: time.Minute})
			if res.Verdict != tt.verdict {
				t.Fatalf("verdict %s, want %s (failure %+v)", res.Verdict, tt.verdict, res.Failure)
			}
			if tt.reason == "" {
				if res.Failure != nil {
					t.Errorf("unexpected failure %+v", res.Failure)
				}
				return
			}
			if res.Failure == nil || res.Failure.Service != tt.service || res.Failure.Reason != tt.reason || res.Failure.Hint == "" {
				t.Errorf("failure %+v, want service %s, reason %s and a hint", res.Failure, tt.service, tt.reason)
			}
		})
	}
}

func TestHints(t *testing.T) {
	t.Parallel()
	res := Evaluate(Input{Services: []string{"db"}, Containers: []docker.Container{
		{ID: "db", Labels: map[string]string{stack.LabelService: "db"}, State: "running", Health: "unhealthy", HealthOutput: "pg_isready: no response"},
	}})
	if want := "its healthcheck fails: pg_isready: no response"; res.Failure.Hint != want {
		t.Errorf("hint %q, want %q", res.Failure.Hint, want)
	}
	res = Evaluate(Input{Services: []string{"api"}, Containers: []docker.Container{ct("api", "exited", "", 1)}})
	if want := "exited with code 1; see its last log lines"; res.Failure.Hint != want {
		t.Errorf("hint %q, want %q", res.Failure.Hint, want)
	}
}
