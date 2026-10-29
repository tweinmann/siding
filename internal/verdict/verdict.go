// Package verdict decides whether a stack runs: Ready only when every service is up, healthy
// where it has a healthcheck, and answering through the proxy where it has a route path.
// Evaluate is a pure function; status and up --wait call it once per poll.
package verdict

import (
	"fmt"
	"time"

	"github.com/tweinmann/siding/internal/docker"
	"github.com/tweinmann/siding/internal/stack"
)

// Verdicts.
const (
	Ready    = "Ready"
	Starting = "Starting"
	Failed   = "Failed"
	TimedOut = "TimedOut"
)

// Reasons a service is not ready. Part of the --json contract.
const (
	Exited       = "Exited"
	Unhealthy    = "Unhealthy"
	Restarting   = "Restarting"
	Missing      = "Missing"
	NotAnswering = "NotAnswering"
	NotReady     = "NotReady"
	SeedFailed   = "SeedFailed"
)

// restartLimit is how many restarts while waiting count as a crash loop.
const restartLimit = 3

// Route is a routed service as the probe sees it.
type Route struct {
	URL  string
	Port int
}

// Input is everything one evaluation looks at.
type Input struct {
	// Services are the compose file's services, in compose order.
	Services   []string
	Containers []docker.Container
	// Routes are the services with a route path.
	Routes map[string]Route
	// Answering holds, per routed service, whether the last probe got its own answer.
	Answering map[string]bool
	// RestartBase is each container's restart count when waiting began, by container ID.
	RestartBase map[string]int
	// Final is set when the time is up: what is not ready now is TimedOut.
	Final   bool
	Timeout time.Duration
	// Seed is set when a seed command of this up failed; it decides the verdict.
	Seed *Failure
}

// Service is the state of one service.
type Service struct {
	Name   string
	State  string // container state, or "missing"
	Health string
	// Answering is set for routed services only.
	Answering *bool
	Ready     bool
}

// Failure names the service that keeps the stack from being Ready.
type Failure struct {
	Service string
	Reason  string
	Hint    string
	// Output replaces the service's log tail, e.g. with a failed seed command's output.
	Output []string
}

// Result is a verdict with the state of every service.
type Result struct {
	Verdict  string
	Services []Service
	Failure  *Failure
}

type outcome int

const (
	ready outcome = iota
	pending
	failed
)

// Evaluate returns the verdict for in.
func Evaluate(in Input) Result {
	byService := map[string][]docker.Container{}
	for _, c := range in.Containers {
		s := c.Labels[stack.LabelService]
		byService[s] = append(byService[s], c)
	}
	var res Result
	var firstFailed, firstPending *Failure
	allReady := true
	for _, name := range in.Services {
		svc, out, why := evaluateService(in, name, byService[name])
		res.Services = append(res.Services, svc)
		switch out {
		case failed:
			if firstFailed == nil {
				firstFailed = why
			}
			allReady = false
		case pending:
			if firstPending == nil {
				firstPending = why
			}
			allReady = false
		}
	}
	switch {
	case in.Seed != nil:
		res.Verdict, res.Failure = Failed, in.Seed
	case firstFailed != nil:
		res.Verdict, res.Failure = Failed, firstFailed
	case allReady:
		res.Verdict = Ready
	case in.Final:
		res.Verdict, res.Failure = TimedOut, firstPending
	default:
		res.Verdict = Starting
	}
	return res
}

// evaluateService looks at every container of a service; the worst one decides.
func evaluateService(in Input, name string, containers []docker.Container) (Service, outcome, *Failure) {
	svc := Service{Name: name}
	route, routed := in.Routes[name]
	if routed {
		answering := in.Answering[name]
		svc.Answering = &answering
	}
	if len(containers) == 0 {
		svc.State = "missing"
		return svc, failed, &Failure{Service: name, Reason: Missing, Hint: "the service has no container; run siding up"}
	}
	worst, why := ready, (*Failure)(nil)
	for _, c := range containers {
		svc.State, svc.Health = c.State, c.Health
		out, f := evaluateContainer(in, name, c, route, routed)
		if out > worst {
			worst, why = out, f
		}
		if worst == failed {
			break
		}
	}
	svc.Ready = worst == ready
	return svc, worst, why
}

func evaluateContainer(in Input, name string, c docker.Container, route Route, routed bool) (outcome, *Failure) {
	failure := func(reason, format string, args ...any) (outcome, *Failure) {
		return failed, &Failure{Service: name, Reason: reason, Hint: fmt.Sprintf(format, args...)}
	}
	notReady := func() (outcome, *Failure) {
		health := ""
		if c.Health != "" {
			health = ", health " + c.Health
		}
		return pending, &Failure{Service: name, Reason: NotReady,
			Hint: fmt.Sprintf("not ready after %s (state %s%s); raise --timeout, or see its last log lines", in.Timeout, c.State, health)}
	}
	restarts := c.RestartCount - in.RestartBase[c.ID]
	switch {
	case c.State == "exited" || c.State == "dead":
		if c.ExitCode == 0 && c.State == "exited" {
			return ready, nil // a one-off service such as a migration has done its job
		}
		if c.OOMKilled {
			return failure(Exited, "exited with code %d, killed for lack of memory; give it more memory or see its last log lines", c.ExitCode)
		}
		return failure(Exited, "exited with code %d; see its last log lines", c.ExitCode)
	case restarts >= restartLimit || (c.State == "restarting" && in.Final):
		return failure(Restarting, "keeps restarting (%d restarts, exit code %d); see its last log lines", c.RestartCount, c.ExitCode)
	case c.Health == "unhealthy":
		if c.HealthOutput != "" {
			return failure(Unhealthy, "its healthcheck fails: %s", c.HealthOutput)
		}
		return failure(Unhealthy, "its healthcheck fails; see its last log lines")
	case c.State != "running" || c.Health == "starting":
		return notReady()
	case routed && !in.Answering[name]:
		return pending, &Failure{Service: name, Reason: NotAnswering,
			Hint: fmt.Sprintf("it runs, but %s does not answer through the proxy; check that it listens on port %d on 0.0.0.0, not only on 127.0.0.1", route.URL, route.Port)}
	}
	return ready, nil
}
