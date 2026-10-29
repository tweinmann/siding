# Spec Delta

## MODIFIED Requirements

### Requirement: Verdict
`siding status` SHALL report one verdict for the worktree's stack: `Ready`, `Starting`,
`Failed`, `TimedOut` or `Idle`, the state of every service, and the stack's address. Without
`--wait` it SHALL report the current state once.

#### Scenario: Status without wait
- **WHEN** `siding status` runs while a healthcheck is still starting
- **THEN** it reports `Starting` and exits 2

#### Scenario: No stack
- **WHEN** `siding status` runs in a worktree whose project has no containers
- **THEN** it fails with `StackNotFound` and the hint `siding up`

### Requirement: Waiting
`siding status --wait` SHALL check until the verdict is `Ready`, `Failed` or `Idle`, at most
for `--timeout` (default 3 minutes). When the time is up it SHALL report `TimedOut`, naming the
first service that is not ready, with reason, hint and log tail: `NotAnswering` for a routed
service that never answered, `NotReady` for one that is still starting.

#### Scenario: Ready in time
- **WHEN** `siding status --wait --timeout 60s` runs and the stack becomes ready after 20 seconds
- **THEN** it reports `Ready` after about 20 seconds and exits 0

#### Scenario: Too slow
- **WHEN** `siding status --wait --timeout 10s` runs and `web` with route `/` never answers
- **THEN** it reports `TimedOut` with service `web`, reason `NotAnswering`, and exits 2

#### Scenario: Asleep
- **WHEN** `siding status --wait` runs while the stack sleeps
- **THEN** it reports `Idle` at once and exits 2, without waking the stack

## ADDED Requirements

### Requirement: Idle
A stack SHALL be `Idle` when it is idle-enabled, none of its containers runs, and none exited
with a code other than 0, 137 or 143 (the codes of a stop). `status` MUST NOT send a request
through the proxy to an `Idle` stack, so that reading the status never wakes it. The document
SHALL carry the stack's address and a hint that a request or `siding up --wait` wakes it.

#### Scenario: Asleep after idle
- **WHEN** `shop-feature-x` was stopped after its idle duration and `siding status --json` runs
- **THEN** it prints a `Status` document with `verdict: Idle`, the stack's `url` and the hint, exits 2, and the stack stays stopped

#### Scenario: A crash is not sleep
- **WHEN** every container of an idle-enabled stack is stopped and `api` exited with code 1
- **THEN** the verdict is `Failed` with service `api`, reason `Exited`

#### Scenario: Not idle-enabled
- **WHEN** every container of a stack started without idle stop was stopped with `docker compose stop`
- **THEN** the verdict is the one v0.3 reports for stopped containers, never `Idle`
