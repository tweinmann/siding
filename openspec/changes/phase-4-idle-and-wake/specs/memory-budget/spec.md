# Spec Delta

## MODIFIED Requirements

### Requirement: up refuses over budget
`siding up` SHALL refuse with `NoCapacity` and exit 3, before starting anything, when the
memory in use by every other siding stack on the machine plus the estimate of this stack
exceeds the budget and stopping idle stacks cannot make it fit (see the capability
`idle-stop`). The message MUST name the budget, the use and the estimate. `--force` starts it
anyway, without stopping any stack.

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

#### Scenario: Stopped stacks use nothing
- **WHEN** a stack held 2 GiB before it was stopped to make room
- **THEN** it counts 0 bytes against the budget while it is stopped
