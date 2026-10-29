# Proposal

## Why

v0.2 keeps the machine usable, but new stacks start empty, every agent has to wait until its
database is filled by hand, and a stack that does not fit is simply refused. With several
agents starting at once, the machine also stalls on parallel image builds. v0.3 makes parallel
branches cheap to start: with data from a snapshot or a seed command, in turn when memory is
short, a few builds at a time, and with estimates learned from what stacks really use.

## What Changes

- `siding snapshot save <name>`, `snapshot ls`, `snapshot rm <name>`: copies of a stack's named
  volumes, kept as Docker volumes labelled `dev.siding.snapshot`, per repository. `save` stops
  the services using the volumes while it copies, so databases are consistent.
- `siding up --seed <snapshot>`: on a stack's first `up`, its volumes start as copies of the
  snapshot. The service label `dev.siding.seed` chooses per service: `volume` (the default with
  `--seed`), `none`, or `script` with `dev.siding.seed.command`, a command siding runs once in
  the service on the first `up`; a failing command gives the verdict reason `SeedFailed`.
- Queue: `up --wait` that does not fit into the budget waits in line, in order of arrival,
  until memory frees or `--timeout` is up (then exit 3). Without `--wait`, `up` still refuses
  with exit 3. `ps`, `siding ui` and the `StackList` document show who waits.
- Build limit: `up` of a project with `build:` services holds one of N build slots while Compose
  runs; `siding setup --builds N` sets N (default: half the CPUs) as a label on the proxy.
- Learned peaks: whenever siding reads a stack's memory, it keeps the highest use per
  repository and service in `~/.siding/peaks.json` (a losable cache); the estimate prefers a
  learned peak with a margin over the 256 MiB default.
- `siding teardown --snapshots` also removes this machine's snapshots; `gc` never does.

## Capabilities

### New Capabilities
- `snapshots`: `siding snapshot save|ls|rm`, how snapshots are named, labelled and copied.
- `seeding`: `up --seed`, `dev.siding.seed` (`volume`, `none`, `script`) and
  `dev.siding.seed.command`, and what "first `up`" means.
- `start-queue`: waiting in line for memory with `up --wait`, its order and timeout.
- `build-limit`: build slots, `setup --builds`.

### Modified Capabilities
- `memory-budget`: the estimate uses learned peaks; a new requirement for learning them.
- `stack-status`: the reason `SeedFailed`.
- `cli-contract`: exit 3 after waiting in line; the error codes `SnapshotNotFound` and
  `SnapshotExists`.
- `proxy`: `setup --builds` and the builds in the `Setup` document; `teardown --snapshots`.
- `stack-list`: the queue in `ps` and in the `StackList` document.
- `stack-gc`: gc never removes snapshots.
- `stack-lifecycle`: `down` finds a stack by its volumes when only its data is left (a bug of
  v0.1 found while testing this phase).

## Impact

- New packages for snapshots and seeding (helper containers through the Docker API), the
  queue and build slots (file locks under `~/.siding`, no service), learned peaks; new
  commands `snapshot save|ls|rm`.
- `internal/compose` reads `build:`; `internal/docker` creates and copies volumes and runs
  `exec` through Compose.
- New labels `dev.siding.snapshot`, `dev.siding.snapshot-volume`, `dev.siding.snapshot-created`,
  `dev.siding.seed.command`,
  `dev.siding.builds`: CLAUDE.md and `openspec/config.yaml` conventions; `~/.siding` holds
  `peaks.json`, `queue/` and `builds/`, all losable.
