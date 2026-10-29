# releases Specification

## Purpose
Defines how siding reaches users who do not build it: static binaries per platform, published
for every version tag, with checksums and instructions to install them.

## Requirements

### Requirement: Distribution build
`just dist` SHALL build, in the devcontainer, static siding binaries for `darwin` and `linux`
on `arm64` and `amd64` into `dist/`, named `siding-<os>-<arch>`, with the version given to it,
and a `SHA256SUMS` file over them.

#### Scenario: A build for a tag
- **WHEN** `just dist v0.5.0` runs in the devcontainer
- **THEN** `dist/` holds four binaries and `SHA256SUMS`, and `dist/siding-linux-arm64 version` reports `v0.5.0`

#### Scenario: Checksums
- **WHEN** a user runs `shasum -a 256 -c SHA256SUMS` next to the downloaded binaries
- **THEN** every binary checks out

### Requirement: Release on a tag
A pushed tag `v<major>.<minor>.<patch>` SHALL make CI run `just test`, build with `just dist`
inside the devcontainer, and publish a GitHub Release named after the tag with the four
binaries and `SHA256SUMS`. A failing check MUST publish nothing.

#### Scenario: Tag v0.5.0
- **WHEN** the maintainer pushes the tag `v0.5.0`
- **THEN** the release `v0.5.0` exists on GitHub with `siding-darwin-arm64`, `siding-darwin-amd64`, `siding-linux-arm64`, `siding-linux-amd64` and `SHA256SUMS`

#### Scenario: A failing check
- **WHEN** `just test` fails for the tagged commit
- **THEN** no release is published

### Requirement: Install instructions
The README SHALL say how to install a released binary on macOS and Linux: download it for the
platform, check it against `SHA256SUMS`, make it executable, put it on the `PATH`, and on macOS
remove the quarantine flag of a downloaded, unsigned binary.

#### Scenario: Install on a Mac
- **WHEN** a user follows the README on an Apple silicon Mac
- **THEN** `siding version` runs and reports `darwin/arm64` and the release's version
