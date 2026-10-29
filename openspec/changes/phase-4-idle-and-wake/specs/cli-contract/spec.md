# Spec Delta

## MODIFIED Requirements

### Requirement: Exit codes
siding SHALL exit 0 on success and when a stack is Ready, 1 when a command fails or a stack is
Failed, 2 when a stack is not Ready in time (TimedOut, or Starting without `--wait`) or sleeps
(Idle), 3 when `up` refuses for lack of memory or waited in line until `--timeout`
(NoCapacity), and 130 without a message when interrupted.

#### Scenario: A verdict decides the exit code
- **WHEN** `siding status --wait` ends with the verdict Failed
- **THEN** it exits 1, and with Ready it exits 0

#### Scenario: Asleep
- **WHEN** `siding status` reports the verdict Idle
- **THEN** it exits 2

#### Scenario: Interrupted
- **WHEN** the user presses Ctrl-C during `siding logs -f`
- **THEN** siding stops docker compose and exits 130 without an error message

#### Scenario: Over budget
- **WHEN** `siding up --wait` waits in line for memory until `--timeout` is up
- **THEN** it exits 3 without starting a container
