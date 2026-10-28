# Spec Delta

## Purpose

Defines the memory budget that keeps the machine usable with many stacks: where it is kept, how
the memory of a new stack is estimated before it starts, and how `up` refuses when it would not
fit.

## ADDED Requirements

### Requirement: Budget on the proxy
The budget SHALL be kept as the label `dev.siding.budget` (bytes) on `siding-proxy`, set with
`siding setup --budget <size>` (`512m`, `12g`, or bytes) and removed with `--budget default`.
Without the label the budget SHALL be 75 % of the memory the Docker daemon reports.

#### Scenario: Set a budget
- **WHEN** `siding setup --budget 12g` runs
- **THEN** `siding-proxy` carries `dev.siding.budget=12884901888`, and running stacks keep running

#### Scenario: Default
- **WHEN** the proxy has no budget label and Docker reports 16 GiB of memory
- **THEN** the budget is 12 GiB

#### Scenario: setup keeps the budget
- **WHEN** `siding setup` runs again without `--budget`
- **THEN** the budget set before stays

### Requirement: Estimate of a stack
The estimate of a stack SHALL be the sum over its services of the memory limit
(`deploy.resources.limits.memory`, else `mem_limit`), or 256 MiB for a service without a
limit; a service scaled to several replicas counts once per replica.

#### Scenario: Limits and defaults
- **WHEN** a stack has `db` with `mem_limit: 1g`, `web` with no limit and `api` with `deploy.resources.limits.memory: 512m`
- **THEN** its estimate is 1 GiB + 256 MiB + 512 MiB

### Requirement: up refuses over budget
`siding up` SHALL refuse with `NoCapacity` and exit 3, before starting anything, when the
memory in use by every other siding stack on the machine plus the estimate of this stack
exceeds the budget. The message MUST name the budget, the use and the estimate. `--force`
starts it anyway.

#### Scenario: Over budget
- **WHEN** the budget is 4 GiB, other stacks use 3.5 GiB and the new stack's estimate is 768 MiB
- **THEN** `siding up` exits 3 with `NoCapacity` and the hint names `siding gc`, `siding down` in another worktree, `siding setup --budget` and `--force`

#### Scenario: Forced
- **WHEN** `siding up --force` runs in the same situation
- **THEN** the stack starts, and a warning on stderr says the budget is exceeded

#### Scenario: The stack itself does not count twice
- **WHEN** `siding up` runs again for a stack that already runs
- **THEN** only the other stacks' memory and this stack's estimate count, not its current use as well

#### Scenario: Within budget
- **WHEN** the other stacks and the estimate fit into the budget
- **THEN** `siding up` behaves as without a budget
