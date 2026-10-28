# Spec Delta

## MODIFIED Requirements

### Requirement: Error document
A command that fails SHALL exit 1, or 3 for `NoCapacity`. With `--json` it SHALL print a
document of kind `Error` with a stable `code` and a human-readable `message`; without `--json`
it SHALL print `error: <message>` on stderr, followed by a hint line when there is one.

#### Scenario: An unknown flag with --json
- **WHEN** `siding version --json --nope` runs
- **THEN** it exits 1 and prints an `Error` document with `code` `InvalidArguments`, even though flag parsing failed before `--json` was read

#### Scenario: A failure without --json
- **WHEN** `siding up` runs outside a git repository
- **THEN** it exits 1 and prints `error: …` and a hint on stderr, and nothing on stdout

#### Scenario: The error codes
- **WHEN** a command fails
- **THEN** its code is one of `InvalidArguments`, `NotAGitRepository`, `NoBranch`, `ComposeFileNotFound`, `ComposeFailed`, `DockerUnavailable`, `ProxyNotRunning`, `PortInUse`, `InvalidRoute`, `AmbiguousPort`, `SlugCollision`, `StackNotFound`, `UnmergedBranch`, `DirtyWorktree`, `NoCapacity`, `NotInteractive`, `SnapshotNotFound`, `SnapshotExists`, or `Internal` for an unexpected error

#### Scenario: No capacity
- **WHEN** `siding up --json` refuses because the stack does not fit into the memory budget
- **THEN** it prints an `Error` document with `code` `NoCapacity` and exits 3

### Requirement: Exit codes
siding SHALL exit 0 on success and when a stack is Ready, 1 when a command fails or a stack is
Failed, 2 when a stack is not Ready in time (TimedOut, or Starting without `--wait`), 3 when
`up` refuses for lack of memory or waited in line until `--timeout` (NoCapacity), and 130
without a message when interrupted.

#### Scenario: A verdict decides the exit code
- **WHEN** `siding status --wait` ends with the verdict Failed
- **THEN** it exits 1, and with Ready it exits 0

#### Scenario: Interrupted
- **WHEN** the user presses Ctrl-C during `siding logs -f`
- **THEN** siding stops docker compose and exits 130 without an error message

#### Scenario: Over budget
- **WHEN** `siding up --wait` waits in line for memory until `--timeout` is up
- **THEN** it exits 3 without starting a container
