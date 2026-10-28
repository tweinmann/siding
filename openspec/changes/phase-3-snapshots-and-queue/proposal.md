# Proposal

> Draft. This change holds the roadmap text only and is marked `skip_specs: true`.
> `/opsx:propose` turns it into a full change: it writes the delta specs, `design.md` and
> `tasks.md`, revises this proposal, and removes `skip_specs` from `.openspec.yaml`.

## Why

New stacks start empty and many at once overload the machine. Seeded data and a queue make
parallel branches cheap to start.

## What Changes

Roadmap v0.3:

- Snapshots of volumes and `--seed` (`dev.siding.seed`: `volume|script|none`).
- A queue for capacity instead of refusing.
- A limit on parallel builds.
- Learned memory peaks, kept in the losable cache under `~/.siding`.

## Capabilities

To be decided at `/opsx:propose`.

## Impact

To be decided at `/opsx:propose`.
