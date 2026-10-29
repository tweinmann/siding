# Spec Delta

## MODIFIED Requirements

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
