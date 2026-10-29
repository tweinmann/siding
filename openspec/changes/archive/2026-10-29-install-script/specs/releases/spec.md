# Spec Delta

## MODIFIED Requirements

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

## ADDED Requirements

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
