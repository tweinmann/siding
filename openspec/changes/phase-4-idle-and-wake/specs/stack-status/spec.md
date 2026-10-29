# Spec Delta

## MODIFIED Requirements

### Requirement: Verdict
`siding status` SHALL report one verdict for the worktree's stack: `Ready`, `Starting`,
`Failed`, `TimedOut` or `Stopped`, the state of every service, and the stack's address.
Without `--wait` it SHALL report the current state once.

#### Scenario: Status without wait
- **WHEN** `siding status` runs while a healthcheck is still starting
- **THEN** it reports `Starting` and exits 2

#### Scenario: No stack
- **WHEN** `siding status` runs in a worktree whose project has no containers
- **THEN** it fails with `StackNotFound` and the hint `siding up`

### Requirement: Waiting
`siding status --wait` SHALL check until the verdict is `Ready`, `Failed` or `Stopped`, at most
for `--timeout` (default 3 minutes). When the time is up it SHALL report `TimedOut`, naming the
first service that is not ready, with reason, hint and log tail: `NotAnswering` for a routed
service that never answered, `NotReady` for one that is still starting.

#### Scenario: Ready in time
- **WHEN** `siding status --wait --timeout 60s` runs and the stack becomes ready after 20 seconds
- **THEN** it reports `Ready` after about 20 seconds and exits 0

#### Scenario: Too slow
- **WHEN** `siding status --wait --timeout 10s` runs and `web` with route `/` never answers
- **THEN** it reports `TimedOut` with service `web`, reason `NotAnswering`, and exits 2

#### Scenario: Stopped
- **WHEN** `siding status --wait` runs while every container of the stack is stopped
- **THEN** it reports `Stopped` at once and exits 2, and starts nothing

## ADDED Requirements

### Requirement: Stopped
A stack SHALL be `Stopped` when none of its containers runs or restarts, every exit code is 0,
137 or 143 (the codes of a stop), and at least one of its services has a route path or a
healthcheck. The `Status` document SHALL carry the stack's address and the hint
`siding up --wait`. `status` MUST NOT probe a `Stopped` stack.

#### Scenario: Stopped to make room
- **WHEN** `siding up` in another worktree stopped `shop-main` to make room, and `siding status --json` runs in the worktree of `shop-main`
- **THEN** it prints a `Status` document with `verdict: Stopped`, the stack's `url` and the hint, and exits 2

#### Scenario: Stopped by hand
- **WHEN** the user ran `docker compose -p shop-main stop`
- **THEN** the verdict is `Stopped` as well

#### Scenario: A crash is not a stop
- **WHEN** every container of a stack is stopped and `api` exited with code 1
- **THEN** the verdict is `Failed` with service `api`, reason `Exited`

#### Scenario: Only one-off services
- **WHEN** a stack has no route path and no healthcheck, and every container exited 0
- **THEN** the verdict is `Ready`, as before
