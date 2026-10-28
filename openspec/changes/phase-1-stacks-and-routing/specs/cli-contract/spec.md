# Spec Delta

## Purpose

Defines what agents and scripts can rely on when they call siding: one versioned JSON document
per command with `--json`, stable error codes and fixed exit codes.

## ADDED Requirements

### Requirement: One JSON document per command
Every siding command SHALL accept `--json` and then print exactly one JSON document on stdout
and nothing else there. The document SHALL carry `apiVersion: siding.dev/v1alpha1` and a
`kind`. Empty fields MUST be left out, never printed as `null`.

#### Scenario: A command with --json
- **WHEN** `siding version --json` runs
- **THEN** stdout holds one document with `apiVersion` `siding.dev/v1alpha1` and `kind` `Version`, and nothing else

#### Scenario: Progress of docker compose
- **WHEN** a command with `--json` runs `docker compose`, which reports progress
- **THEN** that progress goes to stderr, and stdout still holds only the document

#### Scenario: An empty field
- **WHEN** a document has a field without a value, such as a commit unknown to the build
- **THEN** the field is missing from the document

### Requirement: Compatible within a version
Within `siding.dev/v1alpha1` a document field and an error code MUST NOT be removed, renamed
or given another meaning; new fields, kinds and codes MAY be added.

#### Scenario: A field is added
- **WHEN** a later release adds a field to the `Status` document
- **THEN** documents still carry `apiVersion` `siding.dev/v1alpha1` and every earlier field with its meaning

### Requirement: Error document
A command that fails SHALL exit 1. With `--json` it SHALL print a document of kind `Error`
with a stable `code` and a human-readable `message`; without `--json` it SHALL print
`error: <message>` on stderr, followed by a hint line when there is one.

#### Scenario: An unknown flag with --json
- **WHEN** `siding version --json --nope` runs
- **THEN** it exits 1 and prints an `Error` document with `code` `InvalidArguments`, even though flag parsing failed before `--json` was read

#### Scenario: A failure without --json
- **WHEN** `siding up` runs outside a git repository
- **THEN** it exits 1 and prints `error: …` and a hint on stderr, and nothing on stdout

#### Scenario: The error codes
- **WHEN** a command fails
- **THEN** its code is one of `InvalidArguments`, `NotAGitRepository`, `NoBranch`, `ComposeFileNotFound`, `ComposeFailed`, `DockerUnavailable`, `ProxyNotRunning`, `PortInUse`, `InvalidRoute`, `AmbiguousPort`, `SlugCollision`, `StackNotFound`, `UnmergedBranch`, `DirtyWorktree`, or `Internal` for an unexpected error

### Requirement: Exit codes
siding SHALL exit 0 on success and when a stack is Ready, 1 when a command fails or a stack is
Failed, 2 when a stack is not Ready in time (TimedOut, or Starting without `--wait`), 3 for
NoCapacity, which v0.1 never returns, and 130 without a message when interrupted.

#### Scenario: A verdict decides the exit code
- **WHEN** `siding status --wait` ends with the verdict Failed
- **THEN** it exits 1, and with Ready it exits 0

#### Scenario: Interrupted
- **WHEN** the user presses Ctrl-C during `siding logs -f`
- **THEN** siding stops docker compose and exits 130 without an error message

### Requirement: Version
`siding version` SHALL print the version, the commit (when the build knows it), the Go version
and the platform; with `--json` as a document of kind `Version` with the fields `version`,
`commit`, `goVersion` and `platform`.

#### Scenario: Text output
- **WHEN** `siding version` runs in a build of commit `0123456789abcdef…`
- **THEN** it prints one line such as `siding v0.1.0 (0123456789ab) go1.27.3 linux/arm64`

#### Scenario: An argument
- **WHEN** `siding version extra` runs
- **THEN** it fails with `InvalidArguments`
