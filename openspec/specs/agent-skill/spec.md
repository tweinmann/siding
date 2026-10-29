# agent-skill Specification

## Purpose
Defines the Claude Code skill siding ships, so that a coding agent starts its worktree's stack
and waits for the verdict instead of guessing whether its work runs.

## Requirements

### Requirement: siding skill prints the skill
`siding skill` SHALL print the skill (`SKILL.md` with name, description and body) on stdout and
write no file. With `--json` it SHALL print a `Skill` document holding the same text. With
`--install` it SHALL instead write the skill as `<dir>/siding/SKILL.md`, where `<dir>` is
`--dir` or else `.claude/skills` at the worktree's top, replacing an older copy, and write no
other file.

#### Scenario: Install the skill
- **WHEN** a user runs `mkdir -p .claude/skills/siding && siding skill > .claude/skills/siding/SKILL.md`
- **THEN** the file is a valid skill named `siding`, and siding wrote nothing itself

#### Scenario: Install with --install
- **WHEN** `siding skill --install` runs in a worktree of `shop`
- **THEN** `.claude/skills/siding/SKILL.md` at the worktree's top holds the skill, and the output says to commit it so every worktree has it

#### Scenario: Another harness
- **WHEN** `siding skill --install --dir ~/.agents/skills` runs
- **THEN** `~/.agents/skills/siding/SKILL.md` holds the skill, and nothing under `.claude` is written

#### Scenario: JSON
- **WHEN** `siding skill --install --json` runs
- **THEN** it prints a `Skill` document with the text and the `path` it wrote

### Requirement: What the skill teaches
The skill SHALL tell the agent to run `siding up --wait --json` after a change that affects the
running app, to read the verdict and exit code, to fix the named service from the failure's
reason, hint and log tail, to report the address when Ready, and on exit 3 (`NoCapacity`) to
stop and tell the user. It MUST NOT tell the agent to use `--force`, `--volumes`, `--idle`,
`siding gc` or `siding share`.

#### Scenario: The commands in the skill
- **WHEN** the printed skill is read
- **THEN** it names `siding up --wait --json`, `siding status --json`, `siding logs --json` and the exit codes 0, 1, 2 and 3, and none of `--force`, `--volumes`, `--idle`, `gc` or `share`

### Requirement: Stopped stacks and addresses in the skill
The skill SHALL say that `Stopped` means the stack was stopped, for instance to make room, and
that `siding up --wait --json` starts it again; that `siding status --json` gives the stack's
address without starting anything; and that only the addresses siding reports work, never
`localhost:<port>` from the compose file.

#### Scenario: A stopped stack
- **WHEN** the printed skill is read
- **THEN** it names the verdict `Stopped` with exit 2 and `siding up --wait --json` to start the stack again

#### Scenario: The address
- **WHEN** the printed skill is read
- **THEN** it says that `siding status --json` holds the stack's `url`, and that `localhost:<port>` from the compose file does not work under siding

### Requirement: Snippets for the harness
`siding skill --install` SHALL print, after what it wrote, a section for `AGENTS.md` that tells
any agent to use siding as the skill does, and the permissions for siding's reading commands
(`up`, `status`, `logs`, `ps`): as a Claude Code `settings.json` entry and as a plain list for
other harnesses. siding MUST NOT write `AGENTS.md` or any settings file.

#### Scenario: The snippets
- **WHEN** `siding skill --install` has run
- **THEN** its output holds the `AGENTS.md` section and `"Bash(siding up:*)"`, `"Bash(siding status:*)"`, `"Bash(siding logs:*)"` and `"Bash(siding ps:*)"`, and none of `down`, `gc`, `snapshot rm` or `teardown`

#### Scenario: No settings written
- **WHEN** `siding skill --install` runs in a repository with `.claude/settings.json`
- **THEN** that file is byte for byte unchanged

### Requirement: Harness-neutral skill
The skill's text SHALL name no harness: it addresses the agent, so that the same file works
wherever the Agent Skills format is read.

#### Scenario: No harness named
- **WHEN** the printed skill is read
- **THEN** it does not contain "Claude Code"
