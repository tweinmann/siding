package cli

import (
	_ "embed"
	"io"

	"github.com/spf13/cobra"
)

// skillText is the Claude Code skill siding ships, kept in the binary so that text and binary
// never drift apart.
//
//go:embed skill/SKILL.md
var skillText string

func newSkillCommand() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "skill",
		Short: "Print a Claude Code skill that teaches an agent to use siding",
		Long: `Print a Claude Code skill (SKILL.md) that tells an agent to start its stack and wait
for the verdict. siding writes no file; install it with

  mkdir -p .claude/skills/siding && siding skill > .claude/skills/siding/SKILL.md`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if asJSON {
				return printJSON(cmd.OutOrStdout(), skillDocument{header: newHeader(kindSkill), Name: "siding", Text: skillText})
			}
			_, err := io.WriteString(cmd.OutOrStdout(), skillText)
			return err
		},
	}
	registerJSON(cmd, &asJSON)
	return cmd
}
