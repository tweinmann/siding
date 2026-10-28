# Spec Delta

## MODIFIED Requirements

### Requirement: Labels
siding SHALL label every container, network and volume of a stack with `dev.siding.repo`,
`dev.siding.branch` (the branch name, not the slug), `dev.siding.worktree` (absolute path) and
`dev.siding.machine`, and its containers with `dev.siding.gitdir` (the repository's common git
directory) and with `dev.siding.agent` when `--agent` or `SIDING_AGENT` names one. External
networks and volumes MUST NOT be relabelled.

#### Scenario: Labels on a running stack
- **WHEN** `siding up` has run in `/src/shop-login` on branch `feature/login`
- **THEN** every container, the project's networks and its named volumes carry `dev.siding.repo=shop`, `dev.siding.branch=feature/login` and `dev.siding.worktree=/src/shop-login`

#### Scenario: An agent name
- **WHEN** `SIDING_AGENT=claude siding up` runs
- **THEN** the containers also carry `dev.siding.agent=claude`

#### Scenario: The git directory
- **WHEN** `siding up` has run in the linked worktree `/src/shop-login` of the repository `/src/shop`
- **THEN** its containers carry `dev.siding.gitdir=/src/shop/.git`, and its networks and volumes do not, so a stack of siding v0.1 keeps its networks on its next `up`
