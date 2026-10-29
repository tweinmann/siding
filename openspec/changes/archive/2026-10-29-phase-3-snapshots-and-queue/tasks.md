# Tasks

## 1. Foundations

- [x] 1.1 Add the error codes `SnapshotNotFound` and `SnapshotExists`; verify with the error-code table test
- [x] 1.2 Read `build:` in `internal/compose` (a `Build` flag per service); verify with a captured config with and without `build:`
- [x] 1.3 Add to `internal/docker`: create a volume with labels, volumes by label with sizes from `/system/df`, remove a volume, run a helper container (pull if missing, create, start, wait, remove) and `NCPU` from `docker info`; verify with unit tests of the mappings and the helper's container config
- [x] 1.4 Add the new labels (D9) to the conventions in CLAUDE.md and `openspec/config.yaml`; verify `openspec validate --all --strict` passes

## 2. Learned peaks

- [x] 2.1 Implement `~/.siding/peaks.json` in `internal/budget` (read, record max, atomic write, a missing or broken file is empty) and the estimate of D7; verify with table tests for the spec scenarios of `memory-budget`, a broken file and a cap at the limit
- [x] 2.2 Record peaks from `ps`, `checkBudget` and a Ready `status`, and use them in `up`'s estimate; verify with fake-based tests that a peak seen by `ps` changes the next `up`'s estimate
- [x] 2.3 Document learned peaks in the README's budget section; verify the shown path and behaviour match the code

## 3. Snapshots

- [x] 3.1 Implement snapshot naming, labels and the copy plan in a pure package (D1); verify with table tests for names, slugs, repositories and `SnapshotExists`
- [x] 3.2 Implement `siding snapshot save <name> [--force] [--json]`: stop the services that mount the volumes, copy with the helper, start them again also on failure; verify with fake-based tests (order of compose stop, copies, compose start; failure in the middle; no volumes) and a `snapshot.json` golden
- [x] 3.3 Implement `siding snapshot ls [--json]` and `siding snapshot rm <name> [--dry-run] [--json]`; verify with fake-based tests and a `snapshotlist.json` golden
- [x] 3.4 Add `teardown --snapshots` and the snapshots `teardown` names without it; verify with fake-based tests and the updated `teardown*.json` goldens
- [x] 3.5 Document snapshots in the README; verify the shown commands run as written in the devcontainer

## 4. Seeding

- [x] 4.1 Detect a first `up` (D2) and validate `dev.siding.seed` and `dev.siding.seed.command`; verify with fake-based tests for first and later `up`s and the `InvalidArguments` cases
- [x] 4.2 Implement `up --seed <name>` (D3): resolve the snapshot, create the volumes with Compose's labels, copy, then run Compose; a later `up` warns and seeds nothing; verify with fake-based tests per scenario of the spec `seeding`
- [x] 4.3 Implement the seed command (D4) and the reason `SeedFailed` in `internal/verdict`; verify with verdict table tests and fake-based tests of `up` and `up --wait` with a failing command
- [x] 4.4 Document seeding in the README (`--seed`, `dev.siding.seed`, `dev.siding.seed.command`); verify the example labels are accepted by `siding lint`

## 5. Queue

- [x] 5.1 Implement the line in `~/.siding/queue` (D5): enter, leave on every exit path, drop dead entries, the oldest entry first; verify with unit tests using fake pids and a temporary home
- [x] 5.2 Make `up --wait` wait in line when the stack does not fit, recheck every two seconds, print its place on stderr, and exit 3 at `--timeout`; `up` without `--wait` keeps refusing; verify with fake-based tests using the fake clock (memory frees, time is up, order, a killed waiter)
- [x] 5.3 Show the line in `ps` (text and `waiting` in `StackList`) and in `siding ui`; verify with the updated `stacklist.json` golden and the page golden
- [x] 5.4 Document the queue in the README; verify the shown behaviour matches the tests

## 6. Build limit

- [x] 6.1 Implement build slots with `flock` under `~/.siding/builds` (D6); verify with unit tests that N holders exclude the N+1st and that closing a holder frees its slot
- [x] 6.2 Hold a slot around `docker compose up` for projects with `build:`, count waiting against `--timeout`, and add `setup --builds` with the label carried forward and `builds` in `Setup`; verify with fake-based tests and the updated `setup.json` golden
- [x] 6.3 Document the build limit in the README; verify the commands run as written

## 7. End-to-end and integration

- [x] 7.1 Extend `internal/e2e`: save a snapshot of a stack with data, seed a new branch from it and read the data back, a seed command that writes a file and one that fails with `SeedFailed`, `gc` and `down --volumes` leaving the snapshot, `snapshot rm`; a second `up --wait` that waits in line and starts after `down` of the first; two builds with `--builds 1` running one after the other; verify with `just smoke-stacks` and that no `siding-test-*` object is left, snapshots included
- [x] 7.2 Run the flow by hand in the devcontainer and on the Mac (`just build-darwin`): `snapshot save`, `up --seed` in a second worktree, a queue with a small budget, `ps` and `siding ui` showing the line; record the result in the phase summary
  - Result (2026-10-29): devcontainer and macOS with Docker Desktop (arm64) both passed. `save` stopped and restarted the service; the second worktree's `up --seed` served the snapshot's data, and Compose adopted the volume without a warning; with `--budget 500m` a second `up --wait` waited at place 1, shown by `ps` and `siding ui`, and started after `down` of the first. Found: `down` after only `down` (7.4) and four small issues (7.5).
- [x] 7.4 Let `down` find a stack by its volumes when its containers are gone, so `down --volumes` after `down` removes the data (found in 7.2); verify with a fake-based test and the end-to-end down case
- [x] 7.5 Fix the small issues found in 7.2: the README's claim that curl resolves `*.localhost` (macOS curl does not), `up --seed` saying what it seeds, `<1 MiB` for a tiny but non-empty size, and `down` mentioning kept volumes only when there are any; verify with unit tests
- [x] 7.3 Update the status in CLAUDE.md, the README and `openspec/config.yaml` to v0.3; verify `just test` passes

## Workflow follow-up

- Stop and show the result; wait for the maintainer's approval of Phase 3.
- `/opsx:archive` merges the deltas into `openspec/specs/`.
