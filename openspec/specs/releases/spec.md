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
The README SHALL lead with the install script as one command, show how to download and read
the script before running it, and keep the manual way for macOS and Linux: download the binary
for the platform, check it against `SHA256SUMS`, make it executable, put it on the `PATH`, and
on macOS remove the quarantine flag of a binary downloaded in a browser. It SHALL name
`go install` for users with Go.

#### Scenario: Install on a Mac
- **WHEN** a user follows the README on an Apple silicon Mac
- **THEN** `siding version` runs and reports `darwin/arm64` and the release's version

#### Scenario: Without the script
- **WHEN** a user does not want to pipe a script into a shell
- **THEN** the README shows how to download `install.sh`, read it and run it, and the manual steps

### Requirement: Install script
`install.sh` SHALL install siding with one command on macOS and Linux, on arm64 and amd64:
detect the platform, download the binary and `SHA256SUMS` of the latest release, or of
`SIDING_VERSION`, check the binary, and put it into `~/.local/bin`, or `SIDING_INSTALL_DIR`,
without `sudo`. It MUST install nothing when the check fails.

#### Scenario: One line
- **WHEN** a user on an Apple silicon Mac runs `curl -fsSL https://raw.githubusercontent.com/tweinmann/siding/main/install.sh | sh`
- **THEN** `~/.local/bin/siding` is the release's `siding-darwin-arm64`, and the script prints `siding version` and that `siding setup` comes next

#### Scenario: A checksum mismatch
- **WHEN** the downloaded binary does not match its line in `SHA256SUMS`
- **THEN** the script exits non-zero, names the mismatch, and installs nothing

#### Scenario: Not on the PATH
- **WHEN** the install directory is not on the `PATH`
- **THEN** the script says so and gives the line to add to the shell's profile

#### Scenario: A pinned version
- **WHEN** `SIDING_VERSION=v0.5.0` is set
- **THEN** the script installs that release instead of the latest

#### Scenario: Another platform
- **WHEN** the script runs on Windows or on a 32-bit system
- **THEN** it exits non-zero naming the platform and the supported ones, and installs nothing

#### Scenario: Update
- **WHEN** siding is installed and the script runs again after a newer release
- **THEN** the newer binary replaces the old one
