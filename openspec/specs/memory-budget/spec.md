# memory-budget Specification

## Purpose
Defines the memory budget that keeps the machine usable with many stacks: where it is kept, how
the memory of a new stack is estimated before it starts, and how `up` refuses when it would not
fit.

## Requirements

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
The estimate of a stack SHALL be the sum over its services, once per replica, of the learned
peak of the service plus a quarter, capped at its memory limit; without a learned peak, of the
memory limit (`deploy.resources.limits.memory`, else `mem_limit`); without either, 256 MiB.

#### Scenario: Limits and defaults
- **WHEN** a stack has `db` with `mem_limit: 1g`, `web` with no limit and `api` with `deploy.resources.limits.memory: 512m`, and siding has learned no peak
- **THEN** its estimate is 1 GiB + 256 MiB + 512 MiB

#### Scenario: A learned peak
- **WHEN** siding has seen `web` of the repository `shop` use at most 400 MiB, and `web` has no limit
- **THEN** `web` counts 500 MiB

#### Scenario: Capped by the limit
- **WHEN** `db` has `mem_limit: 1g` and a learned peak of 900 MiB
- **THEN** `db` counts 1 GiB, not 1125 MiB

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

### Requirement: Learned peaks
Whenever siding reads the memory of running containers (`ps`, `ui`, `status`, `up`), it SHALL
keep the highest use seen per repository and service in `~/.siding/peaks.json`. The file is a
losable cache: without it, or when it cannot be read, estimates fall back to limits and
defaults, and siding writes a new one.

#### Scenario: A peak is learned
- **WHEN** `siding ps` reads 400 MiB for `web` of `shop`, and later 300 MiB
- **THEN** `peaks.json` holds 400 MiB for `shop/web`

#### Scenario: Branches share what they learn
- **WHEN** siding learned 400 MiB for `web` from the stack of `main`
- **THEN** the estimate for a new stack of `feature/x` of `shop` uses it

#### Scenario: The cache is lost
- **WHEN** `~/.siding/peaks.json` is deleted or holds no valid JSON
- **THEN** siding works as without learned peaks and starts learning anew
