# Design

## Context

v0.2 (archived as `2026-10-28-phase-2-overview-and-budget`) estimates a stack from limits and a
256 MiB default, refuses with exit 3 when it does not fit, starts every stack with empty
volumes, and lets every `up` build at once. The pieces this phase builds on: `budget.Check` and
`checkBudget` in `up`, `a.memory` (one-shot stats in parallel), the proxy's labels as settings
(`dev.siding.budget`), the losable cache `~/.siding`, and `status`'s polling loop. Motivation
and scope: see proposal.md; behaviour: see the delta specs. Constraints from CLAUDE.md: Docker
is the database, no background service, never lose work, the compose file is never edited.

## Goals / Non-Goals

**Goals:**
- Snapshots and seeding built only from Docker volumes, a helper container and `docker compose
  exec`; no file format of siding's own.
- Queue and build slots without a service: the waiting `up` processes coordinate through files
  under `~/.siding`, and a killed process never blocks the others.
- Every new behaviour testable with fakes; `just smoke-stacks` covers save, seed, the queue and
  a build slot against the inner daemon.

**Non-Goals:**
- Sharing snapshots between machines, exporting them as files, incremental snapshots.
- Seeding a stack that already has data (that is `down --volumes`, then `up --seed`).
- Priorities in the queue, or a queue across machines (v0.5 has remote hosts).
- Limiting anything but builds (pulls, starts).

## Decisions

### D1. Snapshots are Docker volumes (maintainer's decision)
A snapshot of repository `<repo>` named `<name>` is one volume per own named volume of the
stack, `siding-snap_<repo-slug>_<name>_<volume-key-slug>` (with `_`, which neither slugs nor
snapshot names contain, so repository `a-b` with snapshot `c` and repository `a` with snapshot
`b-c` never share a volume; and siding creates a volume only after checking that the name is
free, because Docker's volume create silently returns an existing volume of that name, and a
snapshot would then write into another machine's or another snapshot's data, as the end-to-end
test showed with two machines on one daemon), labelled `dev.siding.snapshot=<name>`,
`dev.siding.snapshot-volume=<key>`, `dev.siding.repo`, `dev.siding.machine`, and the time in
`dev.siding.snapshot-created`. Copies run in a short-lived helper container from the image
pinned in the binary (`busybox:1.37.0`, the devcontainer's `SIDING_BUSYBOX_IMAGE`) with
`cp -a /from/. /to/`, created, waited for and removed through the Engine API, pulled first when
missing. `save` stops the services that mount the volumes (`docker compose stop <services>`),
copies, and starts them again (`docker compose start`), also when the copy fails. Alternatives,
asked: tar files in a directory the user picks (portable, but data outside Docker); seeding only
from the default branch's running stack (no command, but one state and only while that stack
exists). A live copy without stopping was rejected: a database copied mid-write may not start.

### D2. "First up" instead of a marker (refines the approved option)
The approved option said siding would mark a seeded stack with a label on its volume. Docker
cannot change a volume's labels after creation, so the marker would need a second volume per
stack. A first `up` is instead detected before Compose runs: the project has neither containers
nor volumes. It is the same moment the marker would have recorded, with nothing to store.

### D3. Seeding before Compose starts
For a first `up --seed <name>`, siding resolves the snapshot (`SnapshotNotFound` otherwise),
then for each own named volume of the normalised config that the snapshot holds and no service
with `dev.siding.seed: none` uses, creates the volume under its Compose name with the labels
Compose sets itself (`com.docker.compose.project`, `com.docker.compose.volume`) plus siding's,
and copies the snapshot into it. Then Compose starts the stack and adopts the volumes. Tried
with Compose 5.6.1 before this proposal: with both Compose labels the volume is adopted
silently; without them Compose warns that it "was not created by Docker Compose".

### D4. The seed command (maintainer's decision: a command in the service)
After `docker compose up -d` of a first `up`, siding waits for each service with
`dev.siding.seed: script` to be running (and healthy, if it has a healthcheck), at most
`--timeout`, then runs `docker compose exec -T <service> sh -c <dev.siding.seed.command>`,
keeping the last 20 output lines. A non-zero exit becomes a failure with reason `SeedFailed`
that `verdict.Evaluate` reports before everything else; `up` without `--wait` then prints that
`Status` document and exits 1 instead of the `Stack` document. Alternatives, asked: a separate
seed service that runs once (needs the user to add a service); no `script` in v0.3.

### D5. The queue (maintainer's decision: wait with `--wait`, else exit 3)
A waiting `up` writes `~/.siding/queue/<unix-nanos>-<pid>.json` (project, pid, estimate,
start time) and removes it when it ends, by any path, `defer` and the signal context included.
Every two seconds it lists the directory, drops entries whose process is gone (`kill -0`),
and only when it is the oldest entry checks the budget again; once the stack fits, it removes its
entry and starts as usual. A non-waiting `up` does not take part: it starts when it fits and
refuses otherwise, so it never waits behind a line it is not in. Alternatives, asked: always
wait (an `up` without `--wait` could hang unexpectedly); wait only with `--queue` (agents would
have to know the flag). Rejected: a lock-holding daemon (a background service).

### D6. Build slots (maintainer's decision: slots around `compose up`)
`internal/compose` reads `build:`. If any service has it, `up` takes one of N files
`~/.siding/builds/slot-<i>.lock` with a non-blocking `flock`, trying again every second and
printing once that it waits; the kernel releases the lock when the process ends, so a killed
`up` frees its slot at once. The slot is held for the whole `docker compose up`, which also
covers the start; precise build-only slots would need siding to run `docker compose build`
itself, which the maintainer rejected. N is the label `dev.siding.builds` on the proxy, set by
`setup --builds`, else half of the Docker daemon's CPUs (`docker info`'s `NCPU`, the VM's on
Docker Desktop), at least 1. Alternatives, asked: siding builds itself, then `up --no-build`;
a fixed N without a setting.

### D7. Learned peaks
`internal/budget` gains a cache `~/.siding/peaks.json`: `{"<repo>/<service>": bytes}`. Every
memory read (`ps`, `ui` through `ps`, `checkBudget`, and `status` once when the verdict is
Ready) records `max(old, now)` per container's repository and service, written atomically by
rename; a lost update between two siding processes only loses a sample. The estimate per
service is `min(peak × 1.25, limit)` with a peak, else the limit, else 256 MiB. Keyed by
repository and service, not branch, so every new branch profits from what `main` used. The
margin covers that one-shot stats see the current use, not the maximum. Alternatives: a peak
per branch (new branches would learn nothing); averaging (underestimates spikes); cgroup
`max_usage` (only cgroup v1).

### D8. teardown and gc
`teardown --snapshots` lists and removes the volumes labelled `dev.siding.snapshot` of this
machine; without the flag it names them. `gc` and `down --volumes` select volumes by Compose
project, which snapshot volumes do not carry, so they cannot remove snapshots; a test pins it.

### D9. Labels, codes, documents
New labels `dev.siding.snapshot`, `dev.siding.snapshot-volume`, `dev.siding.snapshot-created`
(volumes), `dev.siding.seed.command` (service), `dev.siding.builds` (proxy); they go into the
conventions of CLAUDE.md and `openspec/config.yaml`. New error codes `SnapshotNotFound`,
`SnapshotExists`; new kinds `Snapshot` (save, rm) and `SnapshotList`; `Setup` gains `builds`,
`StackList` gains `waiting`, `Teardown` gains `snapshots`. The verdict gains `SeedFailed`.

## Risks / Trade-offs

- [Saving stops the services of a running stack] → `save` says so before it stops them;
  agents rarely save, humans do it between tasks.
- [Copying big volumes takes long] → `cp -a` inside the daemon, no data passes through siding;
  progress is the helper container's runtime, printed every 10 seconds.
- [A peak from a heavy test run inflates estimates] → capped at the limit; `peaks.json` can be
  deleted, it is a cache.
- [Compose changes how it adopts volumes] → the labels are what Compose 5.6 sets itself; the
  end-to-end test seeds a stack and checks that Compose prints no warning.
- [`flock` on a network home directory] → `~/.siding` is local on macOS and Linux desktops;
  noted in the README.
- [Two waiting `up`s on different machines with one remote daemon] → out of scope until v0.5.

## Migration Plan

Nothing to migrate. A proxy of v0.2 has no `dev.siding.builds` label and uses the default; the
next `setup --builds` adds it. `~/.siding` gains `peaks.json`, `queue/` and `builds/` on first
use.
