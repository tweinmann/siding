# Spec Delta

## MODIFIED Requirements

### Requirement: Down
`siding down` SHALL remove the stack's containers and networks by project name, also when the
compose file is gone, and keep its volumes. A stack whose containers are already gone but whose
volumes remain SHALL still be found by the labels of its volumes, so its data can be removed. It
SHALL print what it removed, with `--json` as a `Down` document, and with `--dry-run` remove
nothing.

#### Scenario: Data survives
- **WHEN** `siding down` and then `siding up` run
- **THEN** the stack's named volumes still hold their data

#### Scenario: Dry run
- **WHEN** `siding down --dry-run --json` runs
- **THEN** it prints a `Down` document listing the containers and networks with `dryRun: true`, and everything still runs

#### Scenario: Nothing running
- **WHEN** `siding down` runs in a worktree whose stack has neither containers nor volumes
- **THEN** it fails with `StackNotFound`

#### Scenario: Only the data is left
- **WHEN** `siding down` ran before, and now `siding down --volumes` runs in the same worktree
- **THEN** it finds the stack by its volumes and removes them, with the guard for unmerged and dirty work
