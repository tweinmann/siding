package cli

import (
	_ "embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tweinmann/siding/internal/fail"
)

// skillText is the skill siding ships, in the Agent Skills format, kept in the binary so that
// text and binary never drift apart.
//
//go:embed skill/SKILL.md
var skillText string

// defaultSkillsDir is where skill --install writes without --dir, at the worktree's top.
const defaultSkillsDir = ".claude/skills"

// agentsSection is what skill --install offers for AGENTS.md, which many harnesses read. It
// says what the skill says, in short, and names no command an agent should leave to the user.
const agentsSection = `## Running the app with siding

- After a change the running app depends on, run ` + "`siding up --wait --json`" + ` and read
  ` + "`verdict`" + ` and the exit code: 0 Ready, 1 Failed, 2 TimedOut or Stopped, 3 NoCapacity.
- Failed or TimedOut: fix the service in ` + "`failure`" + ` from its ` + "`hint`" + ` and ` + "`logs`" + `, then run it again.
  Stopped: run it again. NoCapacity: stop and tell the user.
- When Ready, report ` + "`stack.url`" + `. Leave stopping and removing stacks and their data to the user.
- The skill ` + "`siding`" + ` (` + "`siding skill`" + `) has the details.
`

// readingCommands are what an agent needs and may run without asking (design D3).
var readingCommands = []string{"up", "status", "logs", "ps"}

func newSkillCommand(a *app) *cobra.Command {
	var asJSON, install bool
	var dir string
	cmd := &cobra.Command{
		Use:   "skill",
		Short: "Print the skill that teaches a coding agent to use siding, or install it",
		Long: `Print the skill (SKILL.md, in the Agent Skills format) that tells a coding agent to
start its stack and wait for the verdict. Without --install, siding writes no file.

With --install, it writes the skill to <dir>/siding/SKILL.md: .claude/skills at the worktree's
top (commit it, so every worktree has it), or the skills directory of any harness with --dir,
e.g. --dir ~/.claude/skills for every repository. It then prints a section for AGENTS.md and
the permissions for siding's reading commands; it never writes those files.`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if dir != "" && !install {
				return fail.New(fail.InvalidArguments, "--dir needs --install").
					WithHint("siding skill --install --dir <skills directory>")
			}
			doc := skillDocument{header: newHeader(kindSkill), Name: "siding", Text: skillText}
			if !install {
				if asJSON {
					return printJSON(cmd.OutOrStdout(), doc)
				}
				_, err := io.WriteString(cmd.OutOrStdout(), skillText)
				return err
			}
			path, replaced, inRepo, err := a.installSkill(cmd, dir)
			if err != nil {
				return err
			}
			doc.Path = path
			// With --json, stdout holds the one document; the snippets go to stderr.
			out := cmd.OutOrStdout()
			if asJSON {
				if err := printJSON(out, doc); err != nil {
					return err
				}
				out = cmd.ErrOrStderr()
			}
			_, err = io.WriteString(out, installText(path, replaced, inRepo))
			return err
		},
	}
	cmd.Flags().BoolVar(&install, "install", false, "write the skill to <dir>/siding/SKILL.md instead of printing it")
	cmd.Flags().StringVar(&dir, "dir", "", "skills directory of your harness (default: .claude/skills at the worktree's top)")
	registerJSON(cmd, &asJSON)
	return cmd
}

// installSkill writes the skill and reports its path, whether it replaced an older copy, and
// whether it went to the default directory of the repository.
func (a *app) installSkill(cmd *cobra.Command, dir string) (path string, replaced, inRepo bool, err error) {
	switch {
	case dir == "":
		top, err := a.opts.Git.Top(cmd.Context(), a.opts.Dir)
		if err != nil {
			return "", false, false, err
		}
		dir, inRepo = filepath.Join(top, defaultSkillsDir), true
	case dir == "~" || strings.HasPrefix(dir, "~/"):
		dir = filepath.Join(a.opts.Home, strings.TrimPrefix(dir, "~"))
	case !filepath.IsAbs(dir):
		dir = filepath.Join(a.opts.Dir, dir)
	}
	path = filepath.Join(dir, "siding", "SKILL.md")
	if _, err := os.Stat(path); err == nil {
		replaced = true
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", false, false, fmt.Errorf("install the skill: %w", err)
	}
	if err := os.WriteFile(path, []byte(skillText), 0o644); err != nil {
		return "", false, false, fmt.Errorf("install the skill: %w", err)
	}
	return path, replaced, inRepo, nil
}

// installText says what was written, how to share it, and the two snippets of design D3.
func installText(path string, replaced, inRepo bool) string {
	var b strings.Builder
	verb := "Wrote"
	if replaced {
		verb = "Updated"
	}
	fmt.Fprintf(&b, "%s %s\n", verb, path)
	if inRepo {
		fmt.Fprintf(&b, "Commit it, so every worktree has it:\n  git add %s/siding/SKILL.md && git commit -m \"Add the siding skill\"\n", defaultSkillsDir)
	}
	b.WriteString("\nFor harnesses that read AGENTS.md, add this section to it:\n\n")
	b.WriteString(agentsSection)
	var allow, plain []string
	for _, c := range readingCommands {
		allow = append(allow, fmt.Sprintf("%q", "Bash(siding "+c+":*)"))
		plain = append(plain, "siding "+c)
	}
	b.WriteString("\nLet the agent run siding's reading commands without asking.\n")
	fmt.Fprintf(&b, "Claude Code, in .claude/settings.json:\n  {\"permissions\": {\"allow\": [%s]}}\n", strings.Join(allow, ", "))
	fmt.Fprintf(&b, "Other harnesses: allow %s.\n", strings.Join(plain, ", "))
	return b.String()
}
