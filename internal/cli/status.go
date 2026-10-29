package cli

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/tweinmann/siding/internal/compose"
	"github.com/tweinmann/siding/internal/docker"
	"github.com/tweinmann/siding/internal/stack"
	"github.com/tweinmann/siding/internal/verdict"
)

const (
	// defaultTimeout is how long --wait waits by default.
	defaultTimeout = 3 * time.Minute
	// pollInterval is the time between two checks while waiting.
	pollInterval = time.Second
	// logTail is how many log lines a failure carries.
	logTail = 20
)

type waitFlags struct {
	wait    bool
	timeout time.Duration
}

func (w *waitFlags) register(cmd *cobra.Command) {
	cmd.Flags().BoolVar(&w.wait, "wait", false, "wait until the stack is Ready, Failed or Stopped, at most --timeout")
	cmd.Flags().DurationVar(&w.timeout, "timeout", defaultTimeout, "how long --wait waits")
}

func newStatusCommand(a *app) *cobra.Command {
	var asJSON bool
	var file string
	var w waitFlags
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Tell whether the worktree's stack runs: Ready, Starting, Failed, TimedOut or Stopped",
		Long: `Tell whether the worktree's stack runs. Ready means every service has a container,
every healthcheck is healthy, and every service with a route path answers through the proxy.
A failure names the service, the reason, a hint and its last log lines. Stopped means no
container runs and none failed, e.g. after up in another worktree stopped it to make room.

Exit codes: 0 Ready, 1 Failed, 2 TimedOut, Stopped (or Starting without --wait).`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			st, err := a.findStack(ctx)
			if err != nil {
				return err
			}
			port, err := a.proxyPort(ctx)
			if err != nil {
				return err
			}
			_, p, err := a.loadProject(ctx, st.wt.Top, file, st.project)
			if err != nil {
				return err
			}
			doc, err := a.status(ctx, st, p, port, w, nil)
			if err != nil {
				return err
			}
			return printStatus(cmd.OutOrStdout(), doc, asJSON)
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", "", "compose file (default: compose.yaml or another name Compose looks for)")
	w.register(cmd)
	registerJSON(cmd, &asJSON)
	return cmd
}

// status evaluates the stack once, or until it is Ready or Failed or the time is up.
// seedFailure, when set, is a failed seed command of the up that called status; it decides the
// verdict.
func (a *app) status(ctx context.Context, st existing, p *compose.Project, port int, w waitFlags, seedFailure *verdict.Failure) (statusDocument, error) {
	routes, _, err := stack.Routes(p)
	if err != nil {
		return statusDocument{}, err
	}
	routed := map[string]verdict.Route{}
	for _, r := range routes {
		if r.Path != "" {
			routed[r.Service] = verdict.Route{URL: address(st.id.Host(), port, r.Path), Port: r.Port}
		}
	}
	healthchecks := map[string]bool{}
	for name, svc := range p.Services {
		healthchecks[name] = svc.Healthcheck
	}
	start := a.opts.Clock.Now()
	var base map[string]int
	for {
		cs, err := a.opts.Docker.Containers(ctx, map[string]string{stack.LabelProject: st.project})
		if err != nil {
			return statusDocument{}, err
		}
		if base == nil {
			base = map[string]int{}
			for _, c := range cs {
				base[c.ID] = c.RestartCount
			}
		}
		final := w.wait && a.opts.Clock.Now().Sub(start) >= w.timeout
		in := verdict.Input{
			Services: p.ServiceNames(), Containers: cs, Routes: routed, Healthchecks: healthchecks,
			RestartBase: base, Final: final, Timeout: w.timeout, Seed: seedFailure,
		}
		// A stopped stack is not probed: nothing would answer (spec stack-status).
		res := verdict.Evaluate(in)
		if res.Verdict != verdict.Stopped {
			in.Answering = a.probe(ctx, st, routes, cs, port)
			res = verdict.Evaluate(in)
		}
		if !w.wait || final || res.Verdict == verdict.Ready || res.Verdict == verdict.Failed || res.Verdict == verdict.Stopped {
			if res.Verdict == verdict.Ready {
				_, _ = a.memory(ctx, cs) // learns this stack's peaks (design D7)
			}
			return a.statusDocument(ctx, st, p, routes, port, cs, res)
		}
		if err := a.opts.Clock.Sleep(ctx, pollInterval); err != nil {
			return statusDocument{}, err
		}
	}
}

// probe asks the proxy, for every running service with a route path, whether it answers.
func (a *app) probe(ctx context.Context, st existing, routes []stack.Route, cs []docker.Container, port int) map[string]bool {
	running := map[string]bool{}
	for _, c := range cs {
		if c.State == "running" && c.Health != "starting" && c.Health != "unhealthy" {
			running[c.Labels[stack.LabelService]] = true
		}
	}
	answering := map[string]bool{}
	for _, r := range routes {
		if r.Path == "" || !running[r.Service] {
			continue
		}
		resp, err := a.opts.Probe.Get(ctx, "127.0.0.1:"+strconv.Itoa(port), st.id.Host(), r.Path)
		answering[r.Service] = err == nil && resp.Answered(st.project+"/"+r.Service)
	}
	return answering
}

func (a *app) statusDocument(ctx context.Context, st existing, p *compose.Project, routes []stack.Route, port int,
	cs []docker.Container, res verdict.Result) (statusDocument, error) {
	doc := statusDocument{header: newHeader(kindStatus), Verdict: res.Verdict, Stack: st.ref(port)}
	if res.Verdict == verdict.Stopped {
		doc.Hint = stoppedHint
	}
	for _, s := range res.Services {
		ss := serviceStatus{Name: s.Name, State: s.State, Health: s.Health, Answering: s.Answering, Ready: s.Ready}
		if res.Verdict == verdict.Stopped {
			ss.Answering = nil // not asked: a stopped stack is not probed
		}
		if r, ok := stack.Find(routes, s.Name); ok {
			ss.Route = r.Path
			ss.URL = address(st.id.ServiceHost(s.Name), port, "/")
		}
		doc.Services = append(doc.Services, ss)
	}
	if f := res.Failure; f != nil {
		doc.Failure = &failureDocument{Service: f.Service, Reason: f.Reason, Hint: f.Hint}
		r, err := redactor(st.wt.Top, p)
		if err != nil {
			return statusDocument{}, err
		}
		if f.Output != nil {
			doc.Failure.Logs = r.Lines(f.Output)
		} else if containers := byService(cs)[f.Service]; len(containers) > 0 {
			lines, err := a.opts.Docker.Logs(ctx, failing(containers).ID, logTail)
			if err != nil {
				return statusDocument{}, err
			}
			doc.Failure.Logs = r.Lines(lines)
		}
	}
	return doc, nil
}

// stoppedHint is what a Stopped verdict tells: siding stopped it to make room, or someone did.
const stoppedHint = "every container of the stack is stopped, perhaps to make room for another stack; siding up --wait starts it again"

// failing picks the container of a service whose logs explain its failure: one that is not
// running, else the first.
func failing(cs []docker.Container) docker.Container {
	for _, c := range cs {
		if c.State != "running" || c.Health == "unhealthy" {
			return c
		}
	}
	return cs[0]
}

// printStatus prints the verdict and returns its exit code.
func printStatus(w io.Writer, doc statusDocument, asJSON bool) error {
	if asJSON {
		if err := printJSON(w, doc); err != nil {
			return err
		}
	} else if err := printStatusText(w, doc); err != nil {
		return err
	}
	switch doc.Verdict {
	case verdict.Ready:
		return nil
	case verdict.Failed:
		return exitCode(exitFailed)
	default:
		return exitCode(exitTimedOut)
	}
}

func printStatusText(w io.Writer, doc statusDocument) error {
	var b strings.Builder
	fmt.Fprintf(&b, "%s: %s  %s\n", doc.Stack.Project, doc.Verdict, doc.Stack.URL)
	var rows [][]string
	for _, s := range doc.Services {
		state := s.State
		if s.Health != "" {
			state += " (" + s.Health + ")"
		}
		reach := ""
		if s.Answering != nil {
			reach = "not answering"
			if *s.Answering {
				reach = "answering"
			}
		}
		where := s.URL
		if s.Route != "" {
			where += "  route " + s.Route
		}
		rows = append(rows, []string{s.Name, state, reach, where})
	}
	b.WriteString(table("  ", rows))
	if doc.Hint != "" {
		fmt.Fprintf(&b, "\n%s\n", doc.Hint)
	}
	if f := doc.Failure; f != nil {
		fmt.Fprintf(&b, "\n%s: %s, %s\n", f.Service, f.Reason, f.Hint)
		if len(f.Logs) > 0 {
			fmt.Fprintf(&b, "last log lines of %s:\n", f.Service)
			for _, l := range f.Logs {
				fmt.Fprintf(&b, "  | %s\n", l)
			}
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
}
