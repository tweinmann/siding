# Spec Delta

## MODIFIED Requirements

### Requirement: Seed per service
The service label `dev.siding.seed` SHALL choose how a service's volumes are seeded: `volume`
(from the snapshot given with `--seed`, or the snapshot `default` without it; also when the
label is missing), `none` (never), or `script`. A volume used by a service with `none` MUST NOT
be seeded. Another value MUST fail `up` with `InvalidArguments`, naming the service.

#### Scenario: none
- **WHEN** `cache` carries `dev.siding.seed: none` and `up --seed base` runs first
- **THEN** the volume of `cache` starts empty, and the others come from `base`

## ADDED Requirements

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
