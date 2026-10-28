# Proposal

> Draft. This change holds the roadmap text only and is marked `skip_specs: true`.
> `/opsx:propose` turns it into a full change: it writes the delta specs, `design.md` and
> `tasks.md`, revises this proposal, and removes `skip_specs` from `.openspec.yaml`.

## Why

With several stacks running, the human needs one view of what runs, what it costs and what can
go, and the machine needs a limit so it stays usable.

## What Changes

Roadmap v0.2:

- Web overview at `siding.localhost`.
- `siding gc`: git-aware cleanup that never deletes unmerged or dirty work.
- `siding lint`: conventions that make a compose file ready for siding (berth-ready).
- Memory budget: `up` refuses when the budget is exhausted, exit 3 NoCapacity.
- Devcontainer: Docker-outside-of-Docker as an alternative to Docker-in-Docker.

## Capabilities

To be decided at `/opsx:propose`.

## Impact

New commands and a small web server; the devcontainer and the spec `dev-environment`
(footprint, safety rules) for Docker-outside-of-Docker.
