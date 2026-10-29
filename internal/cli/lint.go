package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tweinmann/siding/internal/compose"
	"github.com/tweinmann/siding/internal/git"
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
			var envFindings []lint.Finding
			config := a.opts.Compose.Config
			if wt, err := a.opts.Git.Worktree(ctx, a.opts.Dir); err == nil {
				top = wt.Top
				project = stack.Identity{Repo: wt.RepoName(), Branch: wt.Branch}.Project()
				if envFindings, err = a.lintEnvFiles(ctx, wt, file, project); err != nil {
					return err
				}
				// Compose refuses the normal configuration while an env_file is missing.
				if len(envFindings) > 0 {
					config = a.opts.Compose.UnresolvedConfig
				}
			}
			path, p, err := a.loadConfig(ctx, top, file, project, config)
			if err != nil {
				return err
			}
			findings := append(envFindings, lint.Check(p, project, top)...)
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

// lintEnvFiles warns MissingEnvFile for every env file up would refuse to start without.
func (a *app) lintEnvFiles(ctx context.Context, wt git.Worktree, file, project string) ([]lint.Finding, error) {
	path, err := compose.FindFile(a.opts.Dir, wt.Top, file)
	if err != nil {
		return nil, nil // loadConfig reports it
	}
	missing, err := a.missingEnvFiles(ctx, wt, path, project)
	if err != nil {
		return nil, err
	}
	var out []lint.Finding
	for _, rel := range missing {
		out = append(out, lint.Finding{Rule: lint.MissingEnvFile, Severity: lint.Warning, Subject: "file " + rel,
			Message: fmt.Sprintf("the main worktree %s has %s, untracked, and this worktree lacks it; siding up refuses to start without it", wt.MainTop(), rel),
			Hint:    envFileHint(wt, []string{rel})})
	}
	return out, nil
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
