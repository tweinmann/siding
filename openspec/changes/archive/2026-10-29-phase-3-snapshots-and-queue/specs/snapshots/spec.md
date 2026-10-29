# Spec Delta

## Purpose

Defines snapshots: named copies of a stack's volumes, kept in Docker, from which new stacks of
the same repository can start with data instead of empty.

## ADDED Requirements

### Requirement: Save a snapshot
`siding snapshot save <name>` SHALL copy every own named volume of the worktree's stack into a
new volume per volume, labelled `dev.siding.snapshot=<name>`, `dev.siding.snapshot-volume=<key>`,
`dev.siding.repo` and `dev.siding.machine`. It MUST stop the services that use those volumes
while copying and start them again afterwards.

#### Scenario: Save
- **WHEN** `siding snapshot save base` runs in the stack `shop-main` with the volumes `data` and `uploads`
- **THEN** two snapshot volumes with `dev.siding.snapshot=base` exist, and `db` and `web`, which use them, run again

#### Scenario: No volumes
- **WHEN** the stack has no named volumes
- **THEN** `snapshot save` fails with `InvalidArguments` and says there is nothing to save

### Requirement: Snapshot names
A snapshot name SHALL be unique per repository and consist of lowercase letters, digits and
`-`. `save` MUST fail with `SnapshotExists` when the name is taken, unless `--force` replaces it.

#### Scenario: Taken
- **WHEN** `siding snapshot save base` runs and `base` exists for the repository
- **THEN** it fails with `SnapshotExists`, and with `--force` the old snapshot is replaced

#### Scenario: Another repository
- **WHEN** the repositories `shop` and `blog` both save a snapshot `base`
- **THEN** both exist side by side

### Requirement: List and remove
`siding snapshot ls` SHALL list the snapshots of this machine, of the current repository when
run in one, with name, repository, volumes, size where Docker reports it, and creation time;
`--json` prints a `SnapshotList` document. `siding snapshot rm <name>` SHALL remove one, with
`--dry-run`, and fail with `SnapshotNotFound` for an unknown name.

#### Scenario: List
- **WHEN** `siding snapshot ls --json` runs in the repository `shop` with the snapshots `base` and `big`
- **THEN** it prints a `SnapshotList` document with both and their volumes

#### Scenario: Remove
- **WHEN** `siding snapshot rm big` runs
- **THEN** the volumes of `big` are gone and `base` stays

### Requirement: Snapshots outlive stacks
Snapshots MUST NOT be removed by `down --volumes` or `gc`; only `snapshot rm` and
`teardown --snapshots` remove them.

#### Scenario: The stack goes
- **WHEN** `siding down --volumes --force` removes the stack a snapshot was saved from
- **THEN** the snapshot still exists and can seed a new stack
