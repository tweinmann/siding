package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tweinmann/siding/internal/lint"
	"github.com/tweinmann/siding/internal/stack"
)

func newLintCommand(a *app) *cobra.Command {
	var asJSON bool
	var file string
	cmd := &cobra.Command{
		Use:   "lint",
		Short: "Check a compose file for what breaks parallel stacks",
		Long: `Check the compose file siding would use for what breaks parallel stacks (fixed container,
network or volume names, routes up would refuse) as errors, for what weakens the verdict or is
shared between stacks as warnings, and for what siding changes as info. lint never changes a
file. It exits 1 when there is an error.`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			// The project and worktree siding would use; outside a worktree, the directory.
			top, project := a.opts.Dir, stack.Slug(filepath.Base(a.opts.Dir))
			if wt, err := a.opts.Git.Worktree(ctx, a.opts.Dir); err == nil {
				top = wt.Top
				project = stack.Identity{Repo: wt.RepoName(), Branch: wt.Branch}.Project()
			}
			path, p, err := a.loadProject(ctx, top, file, project)
			if err != nil {
				return err
			}
			findings := lint.Check(p, project, top)
			doc := lintDocument{header: newHeader(kindLintReport), File: path}
			for _, f := range findings {
				doc.Findings = append(doc.Findings, lintFinding(f))
			}
			if asJSON {
				err = printJSON(cmd.OutOrStdout(), doc)
			} else {
				err = printLintText(cmd, doc)
			}
			if err != nil {
				return err
			}
			if lint.HasErrors(findings) {
				return exitCode(exitFailed)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", "", "compose file (default: compose.yaml or another name Compose looks for)")
	registerJSON(cmd, &asJSON)
	return cmd
}

func printLintText(cmd *cobra.Command, doc lintDocument) error {
	var b strings.Builder
	if len(doc.Findings) == 0 {
		fmt.Fprintf(&b, "%s: no findings\n", doc.File)
	}
	count := map[string]int{}
	for _, f := range doc.Findings {
		count[f.Severity]++
		fmt.Fprintf(&b, "%-7s %s, %s: %s\n", f.Severity, f.Rule, f.Subject, f.Message)
		if f.Hint != "" {
			fmt.Fprintf(&b, "        hint: %s\n", f.Hint)
		}
	}
	if len(doc.Findings) > 0 {
		fmt.Fprintf(&b, "%s: %d errors, %d warnings, %d info\n", doc.File, count[lint.Error], count[lint.Warning], count[lint.Info])
	}
	_, err := fmt.Fprint(cmd.OutOrStdout(), b.String())
	return err
}
