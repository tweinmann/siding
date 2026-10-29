package cli

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tweinmann/siding/internal/compose"
	"github.com/tweinmann/siding/internal/fail"
	"github.com/tweinmann/siding/internal/snapshot"
	"github.com/tweinmann/siding/internal/stack"
)

func newSnapshotCommand(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "snapshot",
		Short: "Save, list and remove snapshots of a stack's volumes",
		Long: `Snapshots are copies of a stack's named volumes, kept as Docker volumes per repository. A new
stack of the same repository can start from one with siding up --seed <name>. Snapshots outlive
stacks: down --volumes and gc never remove them.`,
	}
	cmd.AddCommand(newSnapshotSaveCommand(a), newSnapshotLsCommand(a), newSnapshotRmCommand(a))
	return cmd
}

func newSnapshotSaveCommand(a *app) *cobra.Command {
	var asJSON, force bool
	var file string
	cmd := &cobra.Command{
		Use:   "save <name>",
		Short: "Copy the worktree's stack's volumes into a snapshot",
		Long: `Copy every named volume of the worktree's stack into the snapshot <name>. The services that use
those volumes are stopped while copying, so databases are consistent, and started again.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			doc, err := a.snapshotSave(cmd, args[0], file, force)
			if err != nil {
				return err
			}
			if asJSON {
				return printJSON(cmd.OutOrStdout(), doc)
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Saved the snapshot %s of %s: %s\n", doc.Name, doc.Repo, strings.Join(snapshotVolumeKeys(doc), ", "))
			return err
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "replace a snapshot of the same name")
	cmd.Flags().StringVarP(&file, "file", "f", "", "compose file (default: compose.yaml or another name Compose looks for)")
	registerJSON(cmd, &asJSON)
	return cmd
}

func (a *app) snapshotSave(cmd *cobra.Command, name, file string, force bool) (snapshotDocument, error) {
	ctx := cmd.Context()
	if err := snapshot.ValidName(name); err != nil {
		return snapshotDocument{}, err
	}
	st, err := a.findStack(ctx)
	if err != nil {
		return snapshotDocument{}, err
	}
	_, p, err := a.loadProject(ctx, st.wt.Top, file, st.project)
	if err != nil {
		return snapshotDocument{}, err
	}
	var keys []string
	for key, v := range p.Volumes {
		if !v.External {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		return snapshotDocument{}, fail.New(fail.InvalidArguments, "the stack %s has no named volumes: there is nothing to save", st.project)
	}
	existing, err := a.snapshots(ctx, false)
	if err != nil {
		return snapshotDocument{}, err
	}
	if old, err := snapshot.Find(existing, st.id.Repo, name); err == nil {
		if !force {
			return snapshotDocument{}, snapshot.ExistsError(st.id.Repo, name)
		}
		for _, v := range old.Volumes {
			if err := a.opts.Docker.RemoveVolume(ctx, v); err != nil {
				return snapshotDocument{}, err
			}
		}
	}
	machine, err := a.machineID()
	if err != nil {
		return snapshotDocument{}, err
	}

	// Stop the services that write into the volumes, and start them again whatever happens.
	var services []string
	for _, svc := range p.ServiceNames() {
		if slices.ContainsFunc(p.Services[svc].Volumes, func(k string) bool { return slices.Contains(keys, k) }) {
			services = append(services, svc)
		}
	}
	inv := compose.Invocation{Dir: st.wt.Top, Project: st.project}
	stderr := cmd.ErrOrStderr()
	if len(services) > 0 {
		_, _ = fmt.Fprintf(stderr, "Stopping %s while copying their volumes\n", strings.Join(services, ", "))
		if err := a.opts.Compose.Run(ctx, inv, append([]string{"stop"}, services...), stderr, stderr); err != nil {
			return snapshotDocument{}, err
		}
		defer func() {
			_ = a.opts.Compose.Run(context.WithoutCancel(ctx), inv, append([]string{"start"}, services...), stderr, stderr)
		}()
	}

	created := a.opts.Clock.Now()
	doc := snapshotDocument{header: newHeader(kindSnapshot), Name: name, Repo: st.id.Repo, Created: created.UTC().Format("2006-01-02T15:04:05Z")}
	for _, key := range keys {
		to := snapshot.VolumeName(st.id.Repo, name, key)
		if err := a.opts.Docker.CreateVolume(ctx, to, snapshot.Labels(st.id.Repo, name, key, machine, created)); err != nil {
			return snapshotDocument{}, err
		}
		exit, out, err := a.opts.Docker.RunHelper(ctx, snapshot.CopyHelper(p.Volumes[key].Name, to, machine))
		if err == nil && exit != 0 {
			err = fail.New(fail.Internal, "copying %s into the snapshot failed with exit code %d: %s", key, exit, strings.Join(out, "; "))
		}
		if err != nil {
			_ = a.opts.Docker.RemoveVolume(context.WithoutCancel(ctx), to)
			for _, v := range doc.Volumes {
				_ = a.opts.Docker.RemoveVolume(context.WithoutCancel(ctx), v.Snapshot)
			}
			return snapshotDocument{}, err
		}
		doc.Volumes = append(doc.Volumes, snapshotVolume{Key: key, Source: p.Volumes[key].Name, Snapshot: to})
	}
	return doc, nil
}

// snapshots lists the snapshots of this machine, with sizes when withSizes.
func (a *app) snapshots(ctx context.Context, withSizes bool) ([]snapshot.Snapshot, error) {
	if err := a.opts.Docker.Ping(ctx); err != nil {
		return nil, err
	}
	machine, err := a.machineID()
	if err != nil {
		return nil, err
	}
	vols, err := a.opts.Docker.Volumes(ctx, map[string]string{snapshot.LabelName: "", stack.LabelMachine: machine})
	if err != nil {
		return nil, err
	}
	var sizes map[string]int64
	if withSizes {
		if sizes, err = a.opts.Docker.VolumeSizes(ctx); err != nil {
			return nil, err
		}
	}
	return snapshot.Group(vols, sizes), nil
}

func snapshotVolumeKeys(doc snapshotDocument) []string {
	var keys []string
	for _, v := range doc.Volumes {
		keys = append(keys, v.Key)
	}
	return keys
}

// currentRepo is the repository the current directory belongs to, as siding names it: the
// label of the worktree's stack (which --repo may have set), else the main worktree's
// directory. Outside a repository it is "".
func (a *app) currentRepo(ctx context.Context) string {
	if st, err := a.findStack(ctx); err == nil {
		return st.id.Repo
	}
	if wt, err := a.opts.Git.Worktree(ctx, a.opts.Dir); err == nil {
		return wt.RepoName()
	}
	return ""
}

func newSnapshotLsCommand(a *app) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "ls",
		Short: "List the snapshots of this repository, or of every repository outside one",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			all, err := a.snapshots(ctx, true)
			if err != nil {
				return err
			}
			repo := a.currentRepo(ctx)
			doc := snapshotListDocument{header: newHeader(kindSnapshotList)}
			for _, s := range all {
				if repo != "" && s.Repo != repo {
					continue
				}
				e := snapshotEntry{Name: s.Name, Repo: s.Repo, Created: s.Created, SizeBytes: s.Size}
				for key := range s.Volumes {
					e.Volumes = append(e.Volumes, key)
				}
				sort.Strings(e.Volumes)
				doc.Snapshots = append(doc.Snapshots, e)
			}
			if asJSON {
				return printJSON(cmd.OutOrStdout(), doc)
			}
			if len(doc.Snapshots) == 0 {
				_, err := fmt.Fprintln(cmd.OutOrStdout(), "No snapshots. Save one with siding snapshot save <name> in a stack.")
				return err
			}
			rows := [][]string{{"REPO", "NAME", "VOLUMES", "SIZE", "CREATED"}}
			for _, e := range doc.Snapshots {
				size := "?"
				if e.SizeBytes >= 0 {
					size = bytesText(uint64(e.SizeBytes))
				}
				rows = append(rows, []string{e.Repo, e.Name, strings.Join(e.Volumes, ", "), size, e.Created})
			}
			_, err = fmt.Fprint(cmd.OutOrStdout(), table("", rows))
			return err
		},
	}
	registerJSON(cmd, &asJSON)
	return cmd
}

func newSnapshotRmCommand(a *app) *cobra.Command {
	var asJSON, dryRun bool
	cmd := &cobra.Command{
		Use:   "rm <name>",
		Short: "Remove a snapshot of this repository",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			all, err := a.snapshots(ctx, false)
			if err != nil {
				return err
			}
			repo := a.currentRepo(ctx)
			if repo == "" {
				return fail.New(fail.NotAGitRepository, "%s is not inside a git worktree", a.opts.Dir).
					WithHint("run siding snapshot rm in a worktree of the snapshot's repository")
			}
			s, err := snapshot.Find(all, repo, args[0])
			if err != nil {
				return err
			}
			doc := snapshotDocument{header: newHeader(kindSnapshot), Name: s.Name, Repo: s.Repo, Created: s.Created, DryRun: dryRun}
			for _, key := range sortedVolumeKeys(s) {
				doc.Volumes = append(doc.Volumes, snapshotVolume{Key: key, Snapshot: s.Volumes[key]})
				if !dryRun {
					if err := a.opts.Docker.RemoveVolume(ctx, s.Volumes[key]); err != nil {
						return err
					}
				}
			}
			if asJSON {
				return printJSON(cmd.OutOrStdout(), doc)
			}
			verb := "Removed"
			if dryRun {
				verb = "Would remove"
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s the snapshot %s of %s: %s\n", verb, s.Name, s.Repo, strings.Join(snapshotVolumeKeys(doc), ", "))
			return err
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "list what would be removed, remove nothing")
	registerJSON(cmd, &asJSON)
	return cmd
}

func sortedVolumeKeys(s snapshot.Snapshot) []string {
	keys := make([]string, 0, len(s.Volumes))
	for k := range s.Volumes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
