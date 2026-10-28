package cli

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tweinmann/siding/internal/compose"
	"github.com/tweinmann/siding/internal/fail"
	"github.com/tweinmann/siding/internal/git"
	"github.com/tweinmann/siding/internal/stack"
)

type downFlags struct {
	volumes, force, dryRun bool
}

func newDownCommand(a *app) *cobra.Command {
	var asJSON bool
	var f downFlags
	cmd := &cobra.Command{
		Use:   "down",
		Short: "Remove the worktree's stack; its volumes stay unless --volumes",
		Long: `Remove the containers and networks of the worktree's stack. Its volumes, and with them
its data, stay. --volumes removes them too, but refuses while the branch is not merged into the
default branch or the worktree has uncommitted changes, unless --force.`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			doc, err := a.down(cmd, f)
			if err != nil {
				return err
			}
			if asJSON {
				return printJSON(cmd.OutOrStdout(), doc)
			}
			return printDownText(cmd, doc, f.volumes)
		},
	}
	cmd.Flags().BoolVar(&f.volumes, "volumes", false, "also remove the stack's volumes and their data")
	cmd.Flags().BoolVar(&f.force, "force", false, "remove the volumes even if the branch is unmerged or the worktree dirty")
	cmd.Flags().BoolVar(&f.dryRun, "dry-run", false, "list what would be removed, remove nothing")
	registerJSON(cmd, &asJSON)
	return cmd
}

func (a *app) down(cmd *cobra.Command, f downFlags) (downDocument, error) {
	ctx := cmd.Context()
	st, err := a.findStack(ctx)
	if err != nil {
		return downDocument{}, err
	}
	if f.volumes && !f.force {
		if err := a.keepsWork(ctx, st); err != nil {
			return downDocument{}, err
		}
	}
	doc, err := a.downPlan(ctx, st, f)
	if err != nil {
		return downDocument{}, err
	}
	if f.dryRun {
		return doc, nil
	}
	args := []string{"down", "--remove-orphans"}
	if f.volumes {
		args = append(args, "--volumes")
	}
	// By project name, without files: it works even when the compose file is gone.
	inv := compose.Invocation{Dir: st.wt.Top, Project: st.project}
	if err := a.opts.Compose.Run(ctx, inv, args, cmd.ErrOrStderr(), cmd.ErrOrStderr()); err != nil {
		return downDocument{}, err
	}
	return doc, nil
}

// keepsWork refuses to remove data while the worktree has changes or the branch is unmerged.
func (a *app) keepsWork(ctx context.Context, st existing) error {
	dirty, err := a.opts.Git.Dirty(ctx, st.wt.Top)
	if err != nil {
		return err
	}
	if dirty {
		return fail.New(fail.DirtyWorktree, "the worktree %s has uncommitted changes; its volumes stay", st.wt.Top).
			WithHint("commit or stash them first, or remove the data anyway with --force")
	}
	m, err := a.opts.Git.MergeState(ctx, git.Repo{Dir: st.wt.Top}, st.wt.Branch)
	if err != nil {
		return err
	}
	if !m.Merged() {
		into := "no default branch was found (origin/HEAD, main or master)"
		if m.Base != "" {
			into = "it is not merged into " + m.BaseName()
		}
		return fail.New(fail.UnmergedBranch, "the branch %s may hold unmerged work: %s; its volumes stay", st.wt.Branch, into).
			WithHint("merge it first, or remove the data anyway with --force (after a squash merge, git fetch --prune makes it count as merged)")
	}
	return nil
}

// downPlan lists what down removes.
func (a *app) downPlan(ctx context.Context, st existing, f downFlags) (downDocument, error) {
	doc := downDocument{header: newHeader(kindDown), Stack: st.ref(a.shownPort(ctx)), DryRun: f.dryRun}
	byProject := map[string]string{stack.LabelProject: st.project}
	cs, err := a.opts.Docker.Containers(ctx, byProject)
	if err != nil {
		return doc, err
	}
	for _, c := range cs {
		doc.Containers = append(doc.Containers, c.Name)
	}
	ns, err := a.opts.Docker.Networks(ctx, byProject)
	if err != nil {
		return doc, err
	}
	for _, n := range ns {
		doc.Networks = append(doc.Networks, n.Name)
	}
	if f.volumes {
		vs, err := a.opts.Docker.Volumes(ctx, byProject)
		if err != nil {
			return doc, err
		}
		for _, v := range vs {
			doc.Volumes = append(doc.Volumes, v.Name)
		}
	}
	sort.Strings(doc.Containers)
	sort.Strings(doc.Networks)
	sort.Strings(doc.Volumes)
	return doc, nil
}

func printDownText(cmd *cobra.Command, doc downDocument, volumes bool) error {
	var b strings.Builder
	verb := "Removed"
	if doc.DryRun {
		verb = "Would remove"
	}
	fmt.Fprintf(&b, "%s from %s:\n", verb, doc.Stack.Project)
	for _, list := range []struct {
		kind  string
		names []string
	}{{"container", doc.Containers}, {"network", doc.Networks}, {"volume", doc.Volumes}} {
		for _, n := range list.names {
			fmt.Fprintf(&b, "  %s %s\n", list.kind, n)
		}
	}
	if !volumes {
		b.WriteString("Volumes stay; siding down --volumes removes them.\n")
	}
	if doc.DryRun {
		b.WriteString("Dry run: nothing was removed.\n")
	}
	_, err := fmt.Fprint(cmd.OutOrStdout(), b.String())
	return err
}
