# Proposal

## Why

Installing a release takes six manual steps today: find the right file in the browser, download
it and `SHA256SUMS`, check it, `chmod`, move it onto the `PATH`, and on macOS clear the
quarantine flag. Each step is a chance to give up before siding ever ran. This is the first
thing a user does, so it should be one line, before `v0.5.0` is tagged and the manual way
becomes a habit.

## What Changes

- An install script `install.sh` at the repository's root, POSIX `sh`, run as
  `curl -fsSL https://raw.githubusercontent.com/tweinmann/siding/main/install.sh | sh`. It
  detects the platform, downloads the binary and `SHA256SUMS` of the latest release (or of
  `SIDING_VERSION`), refuses on a checksum mismatch, and installs to `~/.local/bin` (or
  `SIDING_INSTALL_DIR`) without `sudo`. It says when that directory is not on the `PATH`, and
  ends with `siding version` and the next step. Running it again updates siding.
- The README's install step leads with the one-liner, shows how to read the script first, keeps
  the manual way as a fallback, and mentions `go install` for Go users.
- `just test` runs a test of the script against a `just dist` build served locally.

## Capabilities

### New Capabilities

None.

### Modified Capabilities
- `releases`: the install instructions lead with the script; the install script itself.

## Impact

- New files: `install.sh`, `hack/test-install.sh`; `justfile` (`test`).
- README: step 1 of "Get started".
- No change to siding's binary, its commands or documents.
