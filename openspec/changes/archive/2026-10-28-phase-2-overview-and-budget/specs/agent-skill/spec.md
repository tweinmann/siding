# Spec Delta

## MODIFIED Requirements

### Requirement: What the skill teaches
The skill SHALL tell the agent to run `siding up --wait --json` after a change that affects the
running app, to read the verdict and exit code, to fix the named service from the failure's
reason, hint and log tail, to report the address when Ready, and on exit 3 (`NoCapacity`) to
stop and tell the user. It MUST NOT tell the agent to use `--force`, `--volumes`, `siding gc`
or `siding share`.

#### Scenario: The commands in the skill
- **WHEN** the printed skill is read
- **THEN** it names `siding up --wait --json`, `siding status --json`, `siding logs --json` and the exit codes 0, 1, 2 and 3, and none of `--force`, `--volumes`, `gc` or `share`
