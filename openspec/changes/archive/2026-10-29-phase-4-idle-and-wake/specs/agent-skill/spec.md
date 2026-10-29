# Spec Delta

## MODIFIED Requirements

### Requirement: What the skill teaches
The skill SHALL tell the agent to run `siding up --wait --json` after a change that affects the
running app, to read the verdict and exit code, to fix the named service from the failure's
reason, hint and log tail, to report the address when Ready, and on exit 3 (`NoCapacity`) to
stop and tell the user. It MUST NOT tell the agent to use `--force`, `--volumes`, `--idle`,
`siding gc` or `siding share`.

#### Scenario: The commands in the skill
- **WHEN** the printed skill is read
- **THEN** it names `siding up --wait --json`, `siding status --json`, `siding logs --json` and the exit codes 0, 1, 2 and 3, and none of `--force`, `--volumes`, `--idle`, `gc` or `share`

## ADDED Requirements

### Requirement: Stopped stacks and addresses in the skill
The skill SHALL say that `Stopped` means the stack was stopped, for instance to make room, and
that `siding up --wait --json` starts it again; that `siding status --json` gives the stack's
address without starting anything; and that only the addresses siding reports work, never
`localhost:<port>` from the compose file.

#### Scenario: A stopped stack
- **WHEN** the printed skill is read
- **THEN** it names the verdict `Stopped` with exit 2 and `siding up --wait --json` to start the stack again

#### Scenario: The address
- **WHEN** the printed skill is read
- **THEN** it says that `siding status --json` holds the stack's `url`, and that `localhost:<port>` from the compose file does not work under siding
