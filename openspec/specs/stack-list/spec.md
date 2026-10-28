# stack-list Specification

## Purpose
Defines `siding ps`, the one view of every stack on the machine: what runs, for which branch
and worktree, at which address, and what memory it takes right now.

## Requirements

### Requirement: Every stack on the machine
`siding ps` SHALL list every stack whose containers carry `dev.siding.repo`, found through the
Docker API from any directory, with repository, branch, state, address, memory in use and
worktree. It needs no git repository and no file of siding's.

#### Scenario: Two worktrees
- **WHEN** stacks run for `shop` on `main` and on `feature/login`
- **THEN** `siding ps` lists both, each with its own address

#### Scenario: A stack whose worktree is gone
- **WHEN** a worktree has been deleted while its stack still runs
- **THEN** `siding ps` still lists the stack, and marks the worktree as missing

#### Scenario: Containers of other projects
- **WHEN** the daemon also runs containers without siding labels
- **THEN** `siding ps` does not list them

### Requirement: Stack state in the list
The state in `ps` SHALL be `running` when every container of the stack runs, `partial` when
some do, and `stopped` when none does. `ps` MUST NOT probe the proxy; the full verdict is
`siding status`.

#### Scenario: A stopped container
- **WHEN** one of three containers of a stack has exited
- **THEN** `siding ps` shows the stack as `partial`

### Requirement: Memory in use
`ps` SHALL show each stack's memory in use, the sum over its running containers as the Docker
API reports it, and the total over all stacks.

#### Scenario: Two stacks
- **WHEN** one stack uses 300 MiB and another 500 MiB
- **THEN** `siding ps` shows 300 MiB and 500 MiB for them and 800 MiB in total

### Requirement: StackList document
With `--json`, `ps` SHALL print one `StackList` document with one entry per stack: project,
repository, branch, worktree, machine, agent, state, address, memory in bytes, and
`worktreeMissing` when the worktree is gone.

#### Scenario: No stacks
- **WHEN** `siding ps --json` runs and no stack exists
- **THEN** it prints a `StackList` document without stacks and exits 0
