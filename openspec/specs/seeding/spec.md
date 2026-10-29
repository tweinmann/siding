# seeding Specification

## Purpose
Defines how a new stack starts with data: its volumes copied from a snapshot, or a seed command
run once in a service, chosen per service with `dev.siding.seed`.

## Requirements

### Requirement: First up
A stack's first `up` SHALL be one where its Compose project has neither containers nor volumes
before siding starts it. Seeding happens only on a first `up`; later `up`s never seed again.

#### Scenario: Second up
- **WHEN** `siding up --seed base` runs for a stack whose volumes exist
- **THEN** nothing is seeded, the data stays, and a warning says the stack already has data and names `siding down --volumes`

### Requirement: Seed from a snapshot
On a first `up --seed <snapshot>`, siding SHALL create each own named volume of the project
that the snapshot holds as a copy of it, before Compose starts the stack, labelled so that
Compose adopts it. Volumes the snapshot lacks start empty. An unknown snapshot MUST fail with
`SnapshotNotFound` before anything starts.

#### Scenario: A new branch with data
- **WHEN** `siding up --seed base --wait` runs in a new worktree of `shop`
- **THEN** the stack's `data` volume holds what `data` held when `base` was saved, and the verdict is Ready

#### Scenario: Unknown snapshot
- **WHEN** `siding up --seed nope` runs
- **THEN** it fails with `SnapshotNotFound`, lists the repository's snapshots in the hint, and starts nothing

### Requirement: Seed per service
The service label `dev.siding.seed` SHALL choose how a service's volumes are seeded: `volume`
(from the snapshot given with `--seed`, or the snapshot `default` without it; also when the
label is missing), `none` (never), or `script`. A volume used by a service with `none` MUST NOT
be seeded. Another value MUST fail `up` with `InvalidArguments`, naming the service.

#### Scenario: none
- **WHEN** `cache` carries `dev.siding.seed: none` and `up --seed base` runs first
- **THEN** the volume of `cache` starts empty, and the others come from `base`

### Requirement: Seed command
A service with `dev.siding.seed: script` SHALL carry `dev.siding.seed.command`, which siding runs
once in the service with `docker compose exec` on the first `up`, as soon as the service is
running and healthy if it has a healthcheck. Without the command label, `up` MUST fail with
`InvalidArguments`.

#### Scenario: A seed command
- **WHEN** `db` carries `dev.siding.seed: script` and `dev.siding.seed.command: "psql -f /seed.sql"`, and the first `up --wait` runs
- **THEN** siding runs the command in `db` once, then reports the verdict

#### Scenario: The command fails
- **WHEN** the seed command exits with code 2
- **THEN** the verdict is `Failed` with service `db`, reason `SeedFailed`, the exit code in the hint and the command's last output lines as the log tail

### Requirement: The default snapshot
A first `up` without `--seed` SHALL seed from the repository's snapshot named `default` when it
exists, as `--seed default` would, and say on stderr that it does. `--seed none` SHALL start the
stack empty. Without a snapshot `default`, a first `up` without `--seed` MUST start empty, as
before.

#### Scenario: A new worktree starts with data
- **WHEN** `shop` has the snapshot `default` and an agent runs `siding up --wait --json` in a new worktree
- **THEN** the stack's volumes are copies of `default`, stderr says it seeded from `default`, and the verdict is Ready

#### Scenario: Empty on purpose
- **WHEN** `shop` has the snapshot `default` and `siding up --seed none` runs first
- **THEN** the stack's volumes start empty

#### Scenario: No default
- **WHEN** `shop` has the snapshot `base` but none named `default`
- **THEN** a first `up` without `--seed` starts empty

#### Scenario: Not a first up
- **WHEN** the stack already has volumes and the snapshot `default` exists
- **THEN** `up` seeds nothing and gives no warning about it
