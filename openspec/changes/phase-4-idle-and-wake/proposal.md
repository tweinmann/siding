# Proposal

> Draft. This change holds the roadmap text only and is marked `skip_specs: true`.
> `/opsx:propose` turns it into a full change: it writes the delta specs, `design.md` and
> `tasks.md`, revises this proposal, and removes `skip_specs` from `.openspec.yaml`.

## Why

Most stacks sit idle most of the time. Stopping them and waking them on request frees memory
without the developer noticing.

## What Changes

Roadmap v0.4:

- Idle stop and wake on request (a standard building block such as Sablier).
- Stopping idle stacks when the budget is under pressure.
- Agent skill: `siding status --json` gives the stack's `url` without restarting it, and the
  agent never gives `localhost:<port>` from the compose file, because siding publishes no ports.

## Capabilities

To be decided at `/opsx:propose`.

## Impact

To be decided at `/opsx:propose`.
