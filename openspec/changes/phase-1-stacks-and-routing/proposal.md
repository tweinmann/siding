# Proposal

> Draft. This change holds the roadmap text only and is marked `skip_specs: true`.
> `/opsx:propose` turns it into a full change: it writes the delta specs, `design.md` and
> `tasks.md`, revises this proposal, and removes `skip_specs` from `.openspec.yaml`.

## Why

siding exists to answer "does what my agent built actually run?". v0.1 is the smallest siding
that answers it: one stack per worktree, reachable by name, with a verdict an agent can wait for.

## What Changes

Roadmap v0.1:

- `siding setup`: the shared proxy `siding-proxy` (Traefik, Docker provider) on the network
  `siding`, and the stable machine id.
- `siding up`: writes `.siding/override.yml` (ports reset, network `siding`, Traefik and
  `dev.siding.*` labels) and runs the user's compose file as project `<repo>-<branch-slug>`.
- `siding ps`: every stack on the machine, read from labels and the Docker API.
- `siding status --wait --json`: the verdict Ready, or Failed with service, reason, hint and
  the last log lines; exit 0 Ready, 1 Failed, 2 TimedOut.
- `siding logs`, `siding open`, `siding down` (refuses unmerged or dirty work unless forced;
  `--dry-run`).
- Hostname and path routing: `<repo>.<branch-slug>.localhost`, `dev.siding.route` (`/`,
  `/<path>`, `false`), `<service>.<repo>.<branch-slug>.localhost` for unrouted HTTP services.
- A Claude Code skill that uses the `--json` documents.

## Capabilities

To be decided at `/opsx:propose`.

## Impact

Go code under `internal/`, the Docker Go SDK as a dependency, new smoke tests in the
devcontainer's inner daemon.
