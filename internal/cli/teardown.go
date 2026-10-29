package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tweinmann/siding/internal/compose"
	"github.com/tweinmann/siding/internal/stack"
)

func newTeardownCommand(a *app) *cobra.Command {
	var asJSON, dryRun, snapshots bool
	cmd := &cobra.Command{
		Use:   "teardown",
		Short: "Remove siding-proxy and its network; stacks keep running",
		Long: `Remove what siding setup created: siding-proxy, its files under ~/.siding/proxy/, and the
network "siding" unless a stack still uses it. Stacks are never touched: they keep running as
plain Compose projects, without the proxy. Remove them first with siding down or siding gc.`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			doc, err := a.teardown(cmd, dryRun, snapshots)
			if err != nil {
				return err
			}
			if asJSON {
				return printJSON(cmd.OutOrStdout(), doc)
			}
			return printTeardownText(cmd, doc)
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "list what would be removed, remove nothing")
	cmd.Flags().BoolVar(&snapshots, "snapshots", false, "also remove every snapshot of this machine")
	registerJSON(cmd, &asJSON)
	return cmd
}

func (a *app) teardown(cmd *cobra.Command, dryRun, withSnapshots bool) (teardownDocument, error) {
	ctx := cmd.Context()
	cfg := a.opts.Proxy
	doc := teardownDocument{header: newHeader(kindTeardown), DryRun: dryRun}
	if err := a.opts.Docker.Ping(ctx); err != nil {
		return doc, err
	}
	proxyContainer, err := a.opts.Docker.Container(ctx, cfg.Container)
	if err != nil {
		return doc, err
	}
	if proxyContainer != nil {
		doc.Container = cfg.Container
	}
	cs, err := a.opts.Docker.Containers(ctx, map[string]string{stack.LabelRepo: ""})
	if err != nil {
		return doc, err
	}
	for _, c := range cs {
		if p := c.Labels[stack.LabelProject]; !slices.Contains(doc.Stacks, p) {
			doc.Stacks = append(doc.Stacks, p)
		}
	}
	sort.Strings(doc.Stacks)
	exists, users, err := a.opts.Docker.NetworkUsers(ctx, cfg.Network)
	if err != nil {
		return doc, err
	}
	users = slices.DeleteFunc(users, func(n string) bool { return n == cfg.Container })
	switch {
	case exists && len(users) == 0:
		doc.Network = cfg.Network
	case exists:
		doc.KeptNetwork = cfg.Network
	}
	snaps, err := a.snapshots(ctx, false)
	if err != nil {
		return doc, err
	}
	for _, s := range snaps {
		if withSnapshots {
			doc.Snapshots = append(doc.Snapshots, s.String())
		} else {
			doc.KeptSnapshots = append(doc.KeptSnapshots, s.String())
		}
	}
	if dryRun {
		return doc, nil
	}
	if withSnapshots {
		for _, s := range snaps {
			for _, v := range s.Volumes {
				if err := a.opts.Docker.RemoveVolume(ctx, v); err != nil {
					return doc, err
				}
			}
		}
	}

	dir := filepath.Join(a.opts.Home, ".siding", "proxy", cfg.Project)
	if doc.Container != "" {
		// By project name: the compose file may be gone, or come from another siding.
		inv := compose.Invocation{Dir: a.opts.Home, Project: cfg.Project}
		if err := a.opts.Compose.Run(ctx, inv, []string{"down", "--remove-orphans"}, cmd.ErrOrStderr(), cmd.ErrOrStderr()); err != nil {
			return doc, err
		}
	}
	if err := os.RemoveAll(dir); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return doc, fmt.Errorf("remove %s: %w", dir, err)
	}
	// ~/.siding/proxy goes too once no other proxy (a test's) has files there.
	_ = os.Remove(filepath.Dir(dir))
	if doc.Network != "" {
		if err := a.opts.Docker.RemoveNetwork(ctx, doc.Network); err != nil {
			return doc, err
		}
	}
	return doc, nil
}

func printTeardownText(cmd *cobra.Command, doc teardownDocument) error {
	var b strings.Builder
	verb := "Removed"
	if doc.DryRun {
		verb = "Would remove"
	}
	if doc.Container == "" && doc.Network == "" && len(doc.Snapshots) == 0 {
		b.WriteString("Nothing to remove: siding-proxy does not run here.\n")
	}
	if doc.Container != "" {
		fmt.Fprintf(&b, "%s container %s\n", verb, doc.Container)
	}
	if doc.Network != "" {
		fmt.Fprintf(&b, "%s network %s\n", verb, doc.Network)
	}
	if doc.KeptNetwork != "" {
		fmt.Fprintf(&b, "Kept network %s: stacks still use it\n", doc.KeptNetwork)
	}
	if len(doc.Stacks) > 0 {
		fmt.Fprintf(&b, "These stacks keep running, without the proxy: %s\nRemove them with siding down in their worktree, or siding gc.\n", strings.Join(doc.Stacks, ", "))
	}
	if len(doc.Snapshots) > 0 {
		fmt.Fprintf(&b, "%s the snapshots %s\n", verb, strings.Join(doc.Snapshots, ", "))
	}
	if len(doc.KeptSnapshots) > 0 {
		fmt.Fprintf(&b, "Kept the snapshots %s; siding teardown --snapshots removes them.\n", strings.Join(doc.KeptSnapshots, ", "))
	}
	if doc.DryRun {
		b.WriteString("Dry run: nothing was removed.\n")
	}
	_, err := fmt.Fprint(cmd.OutOrStdout(), b.String())
	return err
}
