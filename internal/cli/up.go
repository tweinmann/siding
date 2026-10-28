package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tweinmann/siding/internal/budget"
	"github.com/tweinmann/siding/internal/compose"
	"github.com/tweinmann/siding/internal/docker"
	"github.com/tweinmann/siding/internal/fail"
	"github.com/tweinmann/siding/internal/stack"
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
	w.register(cmd)
	registerJSON(cmd, &asJSON)
	return cmd
}

type upFlags struct {
	file, repo, agent string
	force             bool
	compose           []string
}

// checkBudget refuses with NoCapacity when the memory of every other stack plus the estimate of
// this one exceeds the budget; with force it only warns.
func (a *app) checkBudget(ctx context.Context, project string, p *compose.Project, force bool, stderr io.Writer) error {
	proxyContainer, err := a.opts.Docker.Container(ctx, a.opts.Proxy.Container)
	if err != nil {
		return err
	}
	b, err := a.budget(ctx, proxyContainer)
	if err != nil {
		return err
	}
	cs, err := a.opts.Docker.Containers(ctx, map[string]string{stack.LabelRepo: ""})
	if err != nil {
		return err
	}
	// This stack's current use does not count: its estimate replaces it.
	others := slices.DeleteFunc(cs, func(c docker.Container) bool { return c.Labels[stack.LabelProject] == project })
	memory, err := a.memory(ctx, others)
	if err != nil {
		return err
	}
	check := budget.Check{Budget: b, Estimate: budget.Estimate(p)}
	for _, m := range memory {
		check.Used += int64(m)
	}
	if check.Fits() {
		return nil
	}
	msg := fmt.Sprintf("%s needs about %s, and the other stacks use %s of the memory budget %s",
		project, bytesText(uint64(check.Estimate)), bytesText(uint64(check.Used)), bytesText(uint64(check.Budget)))
	if force {
		warnf(stderr, "%s; starting it anyway (--force)", msg)
		return nil
	}
	return fail.New(fail.NoCapacity, "%s", msg).
		WithHint("free memory with siding gc or siding down in another worktree, raise it with siding setup --budget, or start anyway with --force")
}

func (a *app) up(cmd *cobra.Command, f upFlags, w waitFlags, asJSON bool) error {
	ctx := cmd.Context()
	stderr := cmd.ErrOrStderr()
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
	if err := a.checkBudget(ctx, project, p, f.force, stderr); err != nil {
		return err
	}
	override, err := writeOverride(wt.Top, stack.Override(p, id, routes, a.opts.Proxy.Network))
	if err != nil {
		return err
	}

	inv := compose.Invocation{Dir: wt.Top, Project: project, Files: []string{file, override}}
	args := append([]string{"up", "-d", "--remove-orphans"}, f.compose...)
	// Compose's progress, and anything it prints, goes to stderr: stdout is for siding's result.
	if err := a.opts.Compose.Run(ctx, inv, args, stderr, stderr); err != nil {
		return err
	}

	st := existing{wt: wt, id: id, project: project}
	if w.wait {
		doc, err := a.status(ctx, st, p, port, w)
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
