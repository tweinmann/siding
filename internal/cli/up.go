package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/tweinmann/siding/internal/budget"
	"github.com/tweinmann/siding/internal/buildslot"
	"github.com/tweinmann/siding/internal/compose"
	"github.com/tweinmann/siding/internal/docker"
	"github.com/tweinmann/siding/internal/fail"
	"github.com/tweinmann/siding/internal/queue"
	"github.com/tweinmann/siding/internal/seed"
	"github.com/tweinmann/siding/internal/snapshot"
	"github.com/tweinmann/siding/internal/stack"
	"github.com/tweinmann/siding/internal/verdict"
)

func newUpCommand(a *app) *cobra.Command {
	var asJSON bool
	var f upFlags
	var w waitFlags
	cmd := &cobra.Command{
		Use:   "up [-- <docker compose up arguments>]",
		Short: "Start the worktree's stack from its unchanged compose file",
		Long: `Start the worktree's stack: the compose file as the Compose project <repo>-<branch>,
plus .siding/override.yml, which resets every published port and routes the HTTP services
through siding-proxy. The compose file is never edited.

Arguments after -- go to docker compose up, e.g. siding up -- --build web.
With --wait, up ends like siding status --wait, with its verdict and exit code.`,
		Args: func(cmd *cobra.Command, args []string) error {
			if n := cmd.ArgsLenAtDash(); n > 0 || (n < 0 && len(args) > 0) {
				return fail.New(fail.InvalidArguments, "siding up takes no arguments before --, got %q", args[0]).
					WithHint("pass docker compose arguments after --, e.g. siding up -- --build")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if n := cmd.ArgsLenAtDash(); n >= 0 {
				f.compose = args[n:]
			}
			if f.agent == "" && a.opts.Getenv != nil {
				f.agent = a.opts.Getenv("SIDING_AGENT")
			}
			return a.up(cmd, f, w, asJSON)
		},
	}
	cmd.Flags().StringVarP(&f.file, "file", "f", "", "compose file (default: compose.yaml or another name Compose looks for)")
	cmd.Flags().StringVar(&f.repo, "repo", "", "repository name (default: the main worktree's directory name)")
	cmd.Flags().StringVar(&f.agent, "agent", "", "agent name for the label dev.siding.agent (default: $SIDING_AGENT)")
	cmd.Flags().BoolVar(&f.force, "force", false, "start the stack even if it does not fit into the memory budget")
	cmd.Flags().StringVar(&f.seed, "seed", "", "on the stack's first up, start its volumes as copies of this snapshot")
	w.register(cmd)
	registerJSON(cmd, &asJSON)
	return cmd
}

type upFlags struct {
	file, repo, agent string
	force             bool
	seed              string
	compose           []string
}

// fit compares the memory of every other stack plus the estimate of this one with the budget.
func (a *app) fit(ctx context.Context, project, repo string, p *compose.Project) (budget.Check, error) {
	proxyContainer, err := a.opts.Docker.Container(ctx, a.opts.Proxy.Container)
	if err != nil {
		return budget.Check{}, err
	}
	b, err := a.budget(ctx, proxyContainer)
	if err != nil {
		return budget.Check{}, err
	}
	cs, err := a.opts.Docker.Containers(ctx, map[string]string{stack.LabelRepo: ""})
	if err != nil {
		return budget.Check{}, err
	}
	// This stack's current use does not count: its estimate replaces it.
	others := slices.DeleteFunc(cs, func(c docker.Container) bool { return c.Labels[stack.LabelProject] == project })
	memory, err := a.memory(ctx, others)
	if err != nil {
		return budget.Check{}, err
	}
	check := budget.Check{Budget: b, Estimate: budget.EstimateLearned(p, repo, budget.LoadPeaks(a.opts.Home))}
	for _, m := range memory {
		check.Used += int64(m)
	}
	return check, nil
}

func capacityMessage(project string, c budget.Check) string {
	return fmt.Sprintf("%s needs about %s, and the other stacks use %s of the memory budget %s",
		project, bytesText(uint64(c.Estimate)), bytesText(uint64(c.Used)), bytesText(uint64(c.Budget)))
}

const capacityHint = "free memory with siding gc or siding down in another worktree, raise it with siding setup --budget, or start anyway with --force"

// checkBudget lets a stack start when it fits into the budget. Otherwise force starts it with a
// warning, wait puts it in line until it fits or the time is up (spec start-queue), and else
// it refuses with NoCapacity.
func (a *app) checkBudget(ctx context.Context, project, repo string, p *compose.Project, force bool, w waitFlags, stderr io.Writer) error {
	check, err := a.fit(ctx, project, repo, p)
	if err != nil || check.Fits() {
		return err
	}
	switch {
	case force:
		warnf(stderr, "%s; starting it anyway (--force)", capacityMessage(project, check))
		return nil
	case w.wait:
		return a.waitInLine(ctx, project, repo, p, check, w.timeout, stderr)
	}
	return fail.New(fail.NoCapacity, "%s", capacityMessage(project, check)).WithHint(capacityHint)
}

// queueInterval is how often a waiting up looks at the line and the budget again.
const queueInterval = 2 * time.Second

// waitInLine waits until this up is first in line and its stack fits, at most timeout.
func (a *app) waitInLine(ctx context.Context, project, repo string, p *compose.Project, check budget.Check, timeout time.Duration, stderr io.Writer) error {
	line := queue.Open(a.opts.Home, a.opts.Alive)
	start := a.opts.Clock.Now()
	leave, err := line.Enter(queue.Entry{Project: project, PID: a.opts.PID, Estimate: check.Estimate, Since: start})
	if err != nil {
		return err
	}
	defer leave()
	shown := 0
	for {
		entries, err := line.List()
		if err != nil {
			return err
		}
		place := queue.Place(entries, a.opts.PID)
		if place != shown {
			_, _ = fmt.Fprintf(stderr, "Waiting for memory: place %d in line; %s\n", place, capacityMessage(project, check))
			shown = place
		}
		if place == 1 {
			if check, err = a.fit(ctx, project, repo, p); err != nil {
				return err
			}
			if check.Fits() {
				return nil
			}
		}
		if !a.opts.Clock.Now().Before(start.Add(timeout)) {
			return fail.New(fail.NoCapacity, "waited %s at place %d in line, and %s", timeout, place, capacityMessage(project, check)).WithHint(capacityHint)
		}
		if err := a.opts.Clock.Sleep(ctx, queueInterval); err != nil {
			return err
		}
	}
}

func (a *app) up(cmd *cobra.Command, f upFlags, w waitFlags, asJSON bool) error {
	ctx := cmd.Context()
	stderr := cmd.ErrOrStderr()
	start := a.opts.Clock.Now()
	wt, err := a.opts.Git.Worktree(ctx, a.opts.Dir)
	if err != nil {
		return err
	}
	machine, err := a.machineID()
	if err != nil {
		return err
	}
	repo := f.repo
	if repo == "" {
		repo = wt.RepoName()
	}
	id := stack.Identity{Repo: repo, Branch: wt.Branch, Worktree: wt.Top, Machine: machine, Agent: f.agent, GitDir: wt.CommonDir}
	project := id.Project()

	if err := a.opts.Docker.Ping(ctx); err != nil {
		return err
	}
	port, err := a.proxyPort(ctx)
	if err != nil {
		return err
	}
	existingContainers, err := a.opts.Docker.Containers(ctx, map[string]string{stack.LabelProject: project})
	if err != nil {
		return err
	}
	for _, c := range existingContainers {
		other := stack.FromLabels(c.Labels)
		// A plain Compose project without siding's labels is adopted.
		if other.Worktree == "" {
			continue
		}
		if other.Worktree != id.Worktree || other.Branch != id.Branch {
			return fail.New(fail.SlugCollision, "the Compose project %s already belongs to the branch %s in %s", project, other.Branch, other.Worktree).
				WithHint("the branches %q and %q have the same slug; rename one, or pass --repo to tell them apart", other.Branch, id.Branch)
		}
	}

	file, p, err := a.loadProject(ctx, wt.Top, f.file, project)
	if err != nil {
		return err
	}
	routes, warnings, err := stack.Routes(p)
	if err != nil {
		return err
	}
	for _, w := range warnings {
		warnf(stderr, "%s", w)
	}
	plan, snap, err := a.seedPlan(ctx, project, id.Repo, p, f.seed, len(existingContainers), stderr)
	if err != nil {
		return err
	}
	if err := a.checkBudget(ctx, project, id.Repo, p, f.force, w, stderr); err != nil {
		return err
	}
	override, err := writeOverride(wt.Top, stack.Override(p, id, routes, a.opts.Proxy.Network))
	if err != nil {
		return err
	}

	if err := a.seedVolumes(ctx, id, project, p, plan, snap, stderr); err != nil {
		return err
	}

	slot, err := a.buildSlot(ctx, p, start.Add(w.timeout), stderr)
	if err != nil {
		return err
	}
	inv := compose.Invocation{Dir: wt.Top, Project: project, Files: []string{file, override}}
	args := append([]string{"up", "-d", "--remove-orphans"}, f.compose...)
	// Compose's progress, and anything it prints, goes to stderr: stdout is for siding's result.
	err = a.opts.Compose.Run(ctx, inv, args, stderr, stderr)
	slot.Release()
	if err != nil {
		return err
	}
	seedFailure, err := a.runSeeds(ctx, inv, plan, w.timeout, stderr)
	if err != nil {
		return err
	}

	st := existing{wt: wt, id: id, project: project}
	if w.wait || seedFailure != nil {
		doc, err := a.status(ctx, st, p, port, w, seedFailure)
		if err != nil {
			return err
		}
		return printStatus(cmd.OutOrStdout(), doc, asJSON)
	}
	doc := stackDocument{header: newHeader(kindStack), Stack: st.ref(port)}
	for _, r := range routes {
		doc.Services = append(doc.Services, serviceAddress{Name: r.Service, URL: address(id.ServiceHost(r.Service), port, "/"), Route: r.Path, Port: r.Port})
	}
	if asJSON {
		return printJSON(cmd.OutOrStdout(), doc)
	}
	return printStackText(cmd, doc)
}

// writeOverride writes .siding/override.yml and .siding/.gitignore, which ignores the whole
// directory, itself included, so .siding/ never makes the worktree dirty.
func writeOverride(top string, content []byte) (string, error) {
	path := filepath.Join(top, stack.OverrideFile)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("write %s: %w", stack.OverrideFile, err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("# Written by siding: nothing in .siding/ belongs in git.\n*\n"), 0o644); err != nil {
		return "", fmt.Errorf("write .siding/.gitignore: %w", err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return "", fmt.Errorf("write %s: %w", stack.OverrideFile, err)
	}
	return path, nil
}

func printStackText(cmd *cobra.Command, doc stackDocument) error {
	var b strings.Builder
	fmt.Fprintf(&b, "%s is up at %s\n", doc.Stack.Project, doc.Stack.URL)
	var rows [][]string
	for _, s := range doc.Services {
		route := ""
		if s.Route != "" {
			route = "route " + s.Route
		}
		rows = append(rows, []string{s.Name, s.URL, route})
	}
	b.WriteString(table("  ", rows))
	b.WriteString("Wait for the verdict with: siding status --wait\n")
	_, err := fmt.Fprint(cmd.OutOrStdout(), b.String())
	return err
}

// seedPlan validates the seed labels and plans the seeding of a first up (spec seeding); a later
// up seeds nothing. The snapshot is resolved here, before anything starts.
func (a *app) seedPlan(ctx context.Context, project, repo string, p *compose.Project, seedName string, containers int, stderr io.Writer) (seed.Plan, snapshot.Snapshot, error) {
	plan, err := seed.Make(p, seedName != "")
	if err != nil {
		return seed.Plan{}, snapshot.Snapshot{}, err
	}
	vols, err := a.opts.Docker.Volumes(ctx, map[string]string{stack.LabelProject: project})
	if err != nil {
		return seed.Plan{}, snapshot.Snapshot{}, err
	}
	if !seed.FirstUp(containers, len(vols)) {
		if seedName != "" {
			warnf(stderr, "%s already has data, so --seed %s seeds nothing; siding down --volumes removes its data", project, seedName)
		}
		return seed.Plan{}, snapshot.Snapshot{}, nil
	}
	if seedName == "" {
		return plan, snapshot.Snapshot{}, nil
	}
	all, err := a.snapshots(ctx, false)
	if err != nil {
		return seed.Plan{}, snapshot.Snapshot{}, err
	}
	snap, err := snapshot.Find(all, repo, seedName)
	return plan, snap, err
}

// seedVolumes creates the planned volumes as copies of the snapshot, with the labels Compose
// sets itself, so Compose adopts them (design D3). Volumes the snapshot lacks start empty.
func (a *app) seedVolumes(ctx context.Context, id stack.Identity, project string, p *compose.Project, plan seed.Plan, snap snapshot.Snapshot, stderr io.Writer) error {
	var keys []string
	for _, key := range plan.Volumes {
		if _, ok := snap.Volumes[key]; ok {
			keys = append(keys, key)
		}
	}
	if len(keys) > 0 {
		_, _ = fmt.Fprintf(stderr, "Seeding %s from the snapshot %s: %s\n", project, snap.Name, strings.Join(keys, ", "))
	}
	for _, key := range keys {
		from := snap.Volumes[key]
		name := p.Volumes[key].Name
		labels := id.Labels()
		labels["com.docker.compose.project"] = project
		labels["com.docker.compose.volume"] = key
		if err := a.opts.Docker.CreateVolume(ctx, name, labels); err != nil {
			return err
		}
		exit, out, err := a.opts.Docker.RunHelper(ctx, snapshot.CopyHelper(from, name, id.Machine))
		if err == nil && exit != 0 {
			err = fail.New(fail.Internal, "seeding %s from the snapshot %s failed with exit code %d: %s", key, snap.Name, exit, strings.Join(out, "; "))
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// runSeeds runs the seed commands of a first up in their services, each as soon as the service
// is ready, at most timeout altogether. A failure is returned as the verdict's failure.
func (a *app) runSeeds(ctx context.Context, inv compose.Invocation, plan seed.Plan, timeout time.Duration, stderr io.Writer) (*verdict.Failure, error) {
	deadline := a.opts.Clock.Now().Add(timeout)
	for _, r := range plan.Scripts {
		for {
			cs, err := a.opts.Docker.Containers(ctx, map[string]string{stack.LabelProject: inv.Project, stack.LabelService: r.Service})
			if err != nil {
				return nil, err
			}
			ready, broken := false, false
			for _, c := range cs {
				ready = ready || (c.State == "running" && (c.Health == "" || c.Health == "healthy"))
				broken = broken || c.State == "exited" || c.State == "dead" || c.Health == "unhealthy"
			}
			if ready {
				break
			}
			if broken || !a.opts.Clock.Now().Before(deadline) {
				return &verdict.Failure{Service: r.Service, Reason: verdict.SeedFailed,
					Hint: fmt.Sprintf("%s did not become ready, so its seed command did not run; see its last log lines", r.Service)}, nil
			}
			if err := a.opts.Clock.Sleep(ctx, pollInterval); err != nil {
				return nil, err
			}
		}
		_, _ = fmt.Fprintf(stderr, "Seeding %s: %s\n", r.Service, r.Command)
		out := &compose.TailBuffer{Max: logTail}
		if err := a.opts.Compose.Run(ctx, inv, []string{"exec", "-T", r.Service, "sh", "-c", r.Command}, out, out); err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return &verdict.Failure{Service: r.Service, Reason: verdict.SeedFailed, Output: out.Lines(),
				Hint: fmt.Sprintf("its seed command %q failed: %v; fix it and start again with siding down --volumes and siding up", r.Command, err)}, nil
		}
	}
	return nil, nil
}

// buildSlot takes one of the build slots for a project that builds images (spec build-limit),
// waiting for a free one until deadline. A project of images only needs none: the nil slot.
func (a *app) buildSlot(ctx context.Context, p *compose.Project, deadline time.Time, stderr io.Writer) (*buildslot.Slot, error) {
	builds := false
	for _, s := range p.Services {
		builds = builds || s.Build
	}
	if !builds {
		return nil, nil
	}
	proxyContainer, err := a.opts.Docker.Container(ctx, a.opts.Proxy.Container)
	if err != nil {
		return nil, err
	}
	n, err := a.builds(ctx, proxyContainer)
	if err != nil {
		return nil, err
	}
	for told := false; ; told = true {
		slot, ok, err := buildslot.TryAcquire(a.opts.Home, n)
		if err != nil || ok {
			return slot, err
		}
		if !told {
			_, _ = fmt.Fprintf(stderr, "Waiting for a build slot: %d stacks build at once (siding setup --builds)\n", n)
		}
		if !a.opts.Clock.Now().Before(deadline) {
			return nil, fail.New(fail.NoCapacity, "waited for a build slot until --timeout; all %d are taken", n).
				WithHint("raise --timeout, or allow more builds at once with siding setup --builds")
		}
		if err := a.opts.Clock.Sleep(ctx, time.Second); err != nil {
			return nil, err
		}
	}
}
