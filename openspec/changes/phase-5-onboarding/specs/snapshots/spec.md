# Spec Delta

## MODIFIED Requirements

### Requirement: Snapshot names
A snapshot name SHALL be unique per repository and consist of lowercase letters, digits and
`-`. `save` MUST fail with `SnapshotExists` when the name is taken, unless `--force` replaces it.
The name `none` MUST be refused with `InvalidArguments`, because `--seed none` means no
snapshot; the name `default` makes the snapshot the one a first `up` seeds from.

#### Scenario: Taken
- **WHEN** `siding snapshot save base` runs and `base` exists for the repository
- **THEN** it fails with `SnapshotExists`, and with `--force` the old snapshot is replaced

#### Scenario: Another repository
- **WHEN** the repositories `shop` and `blog` both save a snapshot `base`
- **THEN** both exist side by side

#### Scenario: Reserved
- **WHEN** `siding snapshot save none` runs
- **THEN** it fails with `InvalidArguments` and saves nothing

#### Scenario: Default
- **WHEN** `siding snapshot save default` runs
- **THEN** the snapshot `default` exists, and the output says that new stacks of the repository start from it
