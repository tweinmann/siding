package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tweinmann/siding/internal/compose"
	"github.com/tweinmann/siding/internal/docker"
	"github.com/tweinmann/siding/internal/fail"
	"github.com/tweinmann/siding/internal/gc"
	"github.com/tweinmann/siding/internal/git"
	"github.com/tweinmann/siding/internal/stack"
)

func newGCCommand(a *app) *cobra.Command {
	var asJSON, dryRun, yes bool
	cmd := &cobra.Command{
		Use:   "gc",
		Short: "Remove the stacks that can go: merged branches and deleted worktrees",
		Long: `Remove the stacks of this machine that can go:
  - a merged branch with a clean worktree: containers, networks and volumes;
  - a deleted worktree: containers and networks, and the volumes only if the branch is merged.
A branch counts as merged when the default branch has moved past its tip, or when its upstream
branch is gone (as after a squash merge and git fetch --prune). gc never removes git
worktrees or branches, and never touches unmerged or uncommitted work. It asks first; --yes
removes without asking.`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			doc, actions, err := a.gcPlan(ctx)
			if err != nil {
				return err
			}
			if dryRun || len(actions) == 0 {
				doc.DryRun = dryRun
				return printGC(cmd, doc, asJSON)
			}
			if !yes {
				if asJSON || a.opts.IsTerminal == nil || !a.opts.IsTerminal() {
					return fail.New(fail.NotInteractive, "siding gc asks before removing, and nobody can answer here").
						WithHint("list what it would remove with --dry-run, and remove it with --yes")
				}
				doc.DryRun = true
				if err := printGC(cmd, doc, false); err != nil {
					return err
				}
				doc.DryRun = false
				_, _ = fmt.Fprint(cmd.ErrOrStderr(), "Remove these? [y/N] ")
				answer, _ := bufio.NewReader(a.opts.Stdin).ReadString('\n')
				if a := strings.ToLower(strings.TrimSpace(answer)); a != "y" && a != "yes" {
					_, err := fmt.Fprintln(cmd.OutOrStdout(), "Nothing was removed.")
					return err
				}
			}
			for _, act := range actions {
				args := []string{"down", "--remove-orphans"}
				if act.Volumes {
					args = append(args, "--volumes")
				}
				// By project name, from a directory that exists: the worktree may be gone.
				inv := compose.Invocation{Dir: a.opts.Home, Project: act.Project}
				if err := a.opts.Compose.Run(ctx, inv, args, cmd.ErrOrStderr(), cmd.ErrOrStderr()); err != nil {
					return err
				}
			}
			return printGC(cmd, doc, asJSON)
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "list what would be removed, remove nothing")
	cmd.Flags().BoolVar(&yes, "yes", false, "remove without asking")
	registerJSON(cmd, &asJSON)
	return cmd
}

// gcPlan gathers what Docker and git know about every stack and plans gc. The overview uses it
// too, for its "can go" list.
func (a *app) gcPlan(ctx context.Context) (gcDocument, []gc.Action, error) {
	doc := gcDocument{header: newHeader(kindGC)}
	machine, err := a.machineID()
	if err != nil {
		return doc, nil, err
	}
	if err := a.opts.Docker.Ping(ctx); err != nil {
		return doc, nil, err
	}
	cs, err := a.opts.Docker.Containers(ctx, map[string]string{stack.LabelRepo: ""})
	if err != nil {
		return doc, nil, err
	}
	projects := map[string]stack.Identity{}
	for _, c := range cs {
		if _, ok := projects[c.Labels[stack.LabelProject]]; !ok {
			projects[c.Labels[stack.LabelProject]] = stack.FromLabels(c.Labels)
		}
	}
	var facts []gc.Facts
	for project, id := range projects {
		facts = append(facts, a.gcFacts(ctx, project, id))
	}
	actions, kept := gc.Plan(facts, machine)
	for _, act := range actions {
		id := projects[act.Project]
		s := gcStack{Project: act.Project, Repo: id.Repo, Branch: id.Branch, Worktree: id.Worktree, Reason: act.Reason}
		byProject := map[string]string{stack.LabelProject: act.Project}
		containers, err := a.opts.Docker.Containers(ctx, byProject)
		if err != nil {
			return doc, nil, err
		}
		networks, err := a.opts.Docker.Networks(ctx, byProject)
		if err != nil {
			return doc, nil, err
		}
		volumes, err := a.opts.Docker.Volumes(ctx, byProject)
		if err != nil {
			return doc, nil, err
		}
		s.Containers, s.Networks = containerNames(containers), networkNames(networks)
		if act.Volumes {
			s.Volumes = volumeNames(volumes)
		} else {
			s.KeptVolumes = volumeNames(volumes)
		}
		doc.Stacks = append(doc.Stacks, s)
	}
	for _, k := range kept {
		id := projects[k.Project]
		doc.Kept = append(doc.Kept, gcKept{Project: k.Project, Repo: id.Repo, Branch: id.Branch, Worktree: id.Worktree, Reason: k.Reason})
	}
	return doc, actions, nil
}

// gcFacts asks the file system and git about one stack. When git cannot answer, the merge state
// stays unknown, and gc keeps the stack's data.
func (a *app) gcFacts(ctx context.Context, project string, id stack.Identity) gc.Facts {
	f := gc.Facts{Project: project, Branch: id.Branch, Worktree: id.Worktree, Machine: id.Machine}
	repo := git.Repo{GitDir: id.GitDir}
	if _, err := os.Stat(id.Worktree); !errors.Is(err, fs.ErrNotExist) {
		f.WorktreeExists = true
		dirty, err := a.opts.Git.Dirty(ctx, id.Worktree)
		f.Dirty = dirty || err != nil // when in doubt, it holds work
		repo = git.Repo{Dir: id.Worktree}
	}
	if repo.Dir == "" && repo.GitDir == "" {
		return f
	}
	if m, err := a.opts.Git.MergeState(ctx, repo, id.Branch); err == nil {
		f.Merge, f.MergeKnown = m, true
	}
	return f
}

func containerNames(cs []docker.Container) []string {
	var out []string
	for _, c := range cs {
		out = append(out, c.Name)
	}
	sort.Strings(out)
	return out
}

func networkNames(ns []docker.Network) []string {
	var out []string
	for _, n := range ns {
		out = append(out, n.Name)
	}
	sort.Strings(out)
	return out
}

func volumeNames(vs []docker.Volume) []string {
	var out []string
	for _, v := range vs {
		out = append(out, v.Name)
	}
	sort.Strings(out)
	return out
}

func printGC(cmd *cobra.Command, doc gcDocument, asJSON bool) error {
	if asJSON {
		return printJSON(cmd.OutOrStdout(), doc)
	}
	var b strings.Builder
	if len(doc.Stacks) == 0 {
		b.WriteString("Nothing to remove.\n")
	}
	verb := "Removed"
	if doc.DryRun {
		verb = "Would remove"
	}
	for _, s := range doc.Stacks {
		fmt.Fprintf(&b, "%s %s (%s, %s):\n", verb, s.Project, s.Branch, s.Reason)
		for _, list := range []struct {
			kind  string
			names []string
		}{{"container", s.Containers}, {"network", s.Networks}, {"volume", s.Volumes}} {
			for _, n := range list.names {
				fmt.Fprintf(&b, "  %s %s\n", list.kind, n)
			}
		}
		for _, v := range s.KeptVolumes {
			fmt.Fprintf(&b, "  keeps volume %s: the branch may hold unmerged work\n", v)
		}
	}
	if len(doc.Kept) > 0 {
		b.WriteString("Kept:\n")
		var rows [][]string
		for _, k := range doc.Kept {
			rows = append(rows, []string{k.Project, k.Branch, k.Reason})
		}
		b.WriteString(table("  ", rows))
	}
	_, err := fmt.Fprint(cmd.OutOrStdout(), b.String())
	return err
}
