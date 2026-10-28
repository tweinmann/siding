# Spec Delta

## MODIFIED Requirements

### Requirement: StackList document
With `--json`, `ps` SHALL print one `StackList` document with one entry per stack: project,
repository, branch, worktree, machine, agent, state, address, memory in bytes, and
`worktreeMissing` when the worktree is gone; the total memory and the budget in bytes; and the
stacks waiting in line, with project, place, memory needed and since when.

#### Scenario: No stacks
- **WHEN** `siding ps --json` runs and no stack exists
- **THEN** it prints a `StackList` document without stacks, with the budget, and exits 0

#### Scenario: A stack waits
- **WHEN** `shop-feature-x` waits in line for memory
- **THEN** the `StackList` document lists it under `waiting` with place 1
