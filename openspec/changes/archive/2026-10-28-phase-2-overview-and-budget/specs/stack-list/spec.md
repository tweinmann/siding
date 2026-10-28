# Spec Delta

## MODIFIED Requirements

### Requirement: Memory in use
`ps` SHALL show each stack's memory in use, the sum over its running containers as the Docker
API reports it, and the total over all stacks against the memory budget.

#### Scenario: Two stacks
- **WHEN** one stack uses 300 MiB and another 500 MiB
- **THEN** `siding ps` shows 300 MiB and 500 MiB for them and 800 MiB in total

#### Scenario: The budget
- **WHEN** the budget is 12 GiB and the stacks use 800 MiB
- **THEN** the total line of `siding ps` reads `800 MiB of 12.0 GiB`

### Requirement: StackList document
With `--json`, `ps` SHALL print one `StackList` document with one entry per stack: project,
repository, branch, worktree, machine, agent, state, address, memory in bytes, and
`worktreeMissing` when the worktree is gone; and the total memory and the budget in bytes.

#### Scenario: No stacks
- **WHEN** `siding ps --json` runs and no stack exists
- **THEN** it prints a `StackList` document without stacks, with the budget, and exits 0
