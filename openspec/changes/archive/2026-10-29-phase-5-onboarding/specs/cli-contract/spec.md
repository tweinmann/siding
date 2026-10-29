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
- **THEN** its code is one of `InvalidArguments`, `NotAGitRepository`, `NoBranch`, `ComposeFileNotFound`, `ComposeFailed`, `DockerUnavailable`, `ProxyNotRunning`, `PortInUse`, `InvalidRoute`, `AmbiguousPort`, `SlugCollision`, `StackNotFound`, `UnmergedBranch`, `DirtyWorktree`, `NoCapacity`, `NotInteractive`, `SnapshotNotFound`, `SnapshotExists`, `EnvFileMissing`, or `Internal` for an unexpected error

#### Scenario: No capacity
- **WHEN** `siding up --json` refuses because the stack does not fit into the memory budget
- **THEN** it prints an `Error` document with `code` `NoCapacity` and exits 3
