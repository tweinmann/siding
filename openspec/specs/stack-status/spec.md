# stack-status Specification

## Purpose
Defines the verdict on a stack that agents and humans wait for: Ready only when it actually
runs and answers through the proxy, otherwise the service that failed, the reason, a hint and
the last lines of its log.

## Requirements

### Requirement: Verdict
`siding status` SHALL report one verdict for the worktree's stack: `Ready`, `Starting`,
`Failed` or `TimedOut`, the state of every service, and the stack's address. Without `--wait`
it SHALL report the current state once.

#### Scenario: Status without wait
- **WHEN** `siding status` runs while a healthcheck is still starting
- **THEN** it reports `Starting` and exits 2

#### Scenario: No stack
- **WHEN** `siding status` runs in a worktree whose project has no containers
- **THEN** it fails with `StackNotFound` and the hint `siding up`

### Requirement: Ready
A stack SHALL be `Ready` only when every service of the compose file has a container, every
service with a healthcheck is healthy, every other service is running or has exited 0, and
every service with a route path answers through the proxy.

#### Scenario: Healthy and answering
- **WHEN** `db` is healthy, `web` with `dev.siding.route: "/"` runs and answers through the proxy, and `migrate` has exited 0
- **THEN** the verdict is `Ready`

#### Scenario: Running but not answering
- **WHEN** `web` with `dev.siding.route: "/"` runs but its process does not listen yet
- **THEN** the verdict is `Starting`, not `Ready`

### Requirement: Answering through the proxy
A routed service SHALL count as answering when a request to its route through the proxy gets
an HTTP response from the service itself, of any status; a response the proxy produces on its
own (no router yet, 502, 503, 504) MUST NOT count.

#### Scenario: The app answers 404
- **WHEN** `api` with `dev.siding.route: "/api"` answers `/api` with 404
- **THEN** `api` counts as answering

#### Scenario: Bad gateway
- **WHEN** the proxy answers 502 for `/api` because nothing listens on the container port
- **THEN** `api` does not count as answering

### Requirement: Failed
A stack SHALL be `Failed` as soon as a service has exited with a non-zero code, is unhealthy,
keeps restarting, has no container, or its seed command failed; the failure MUST name the
service, a reason (`Exited`, `Unhealthy`, `Restarting`, `Missing`, `NotAnswering`,
`SeedFailed`), a hint, and the last 20 log lines of that service or of its seed command.

#### Scenario: A service exits
- **WHEN** `api` exits with code 1 after printing `cannot connect to db`
- **THEN** the verdict is `Failed` with service `api`, reason `Exited`, the exit code in the hint, and the log tail containing `cannot connect to db`

#### Scenario: Unhealthy
- **WHEN** `db`'s healthcheck reports unhealthy
- **THEN** the verdict is `Failed` with reason `Unhealthy`, and the hint contains the last healthcheck output

#### Scenario: Seed failed
- **WHEN** the seed command of `db` exits with code 2 on the first `up --wait`
- **THEN** the verdict is `Failed` with service `db`, reason `SeedFailed`, and the command's output as the log tail

### Requirement: Waiting
`siding status --wait` SHALL check until the verdict is `Ready` or `Failed`, at most for
`--timeout` (default 3 minutes). When the time is up it SHALL report `TimedOut`, naming the
first service that is not ready, with reason, hint and log tail: `NotAnswering` for a routed
service that never answered, `NotReady` for one that is still starting.

#### Scenario: Ready in time
- **WHEN** `siding status --wait --timeout 60s` runs and the stack becomes ready after 20 seconds
- **THEN** it reports `Ready` after about 20 seconds and exits 0

#### Scenario: Too slow
- **WHEN** `siding status --wait --timeout 10s` runs and `web` with route `/` never answers
- **THEN** it reports `TimedOut` with service `web`, reason `NotAnswering`, and exits 2

### Requirement: Status document
With `--json`, `status` SHALL print one `Status` document: the verdict, the stack (project,
repository, branch, worktree, address), every service (state, health, route, address,
answering), and for `Failed` and `TimedOut` the failure with service, reason, hint and log tail.

#### Scenario: An agent reads the failure
- **WHEN** `siding status --wait --json` ends with `Failed`
- **THEN** the document holds `verdict: Failed` and a `failure` with `service`, `reason`, `hint` and `logs`, and the process exits 1
