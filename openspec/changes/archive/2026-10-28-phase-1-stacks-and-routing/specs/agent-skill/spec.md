# Spec Delta

## Purpose

Defines the Claude Code skill siding ships, so that a coding agent starts its worktree's stack
and waits for the verdict instead of guessing whether its work runs.

## ADDED Requirements

### Requirement: siding skill prints the skill
`siding skill` SHALL print a Claude Code skill (`SKILL.md` with name, description and body) on
stdout and write no file. With `--json` it SHALL print a `Skill` document holding the same text.

#### Scenario: Install the skill
- **WHEN** a user runs `mkdir -p .claude/skills/siding && siding skill > .claude/skills/siding/SKILL.md`
- **THEN** the file is a valid skill named `siding`, and siding wrote nothing itself

### Requirement: What the skill teaches
The skill SHALL tell the agent to run `siding up --wait --json` after a change that affects the
running app, to read the verdict and exit code, to fix the named service from the failure's
reason, hint and log tail, and to report the address when Ready. It MUST NOT tell the agent to
use `--force`, `--volumes` or `siding share`.

#### Scenario: The commands in the skill
- **WHEN** the printed skill is read
- **THEN** it names `siding up --wait --json`, `siding status --json`, `siding logs --json` and the exit codes 0, 1 and 2, and none of `--force`, `--volumes` or `share`
