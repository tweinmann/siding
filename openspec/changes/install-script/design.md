# Design

## Context

v0.5 builds `siding-<os>-<arch>` and `SHA256SUMS` with `just dist`, and the release workflow
publishes them on a `v*` tag (spec `releases`). GitHub serves the newest release's assets at
the stable URL `https://github.com/<owner>/<repo>/releases/latest/download/<asset>` and a given
release's at `…/releases/download/<tag>/<asset>`, with no API call and no token. Motivation and
scope: see proposal.md.

## Goals / Non-Goals

**Goals:**
- One command installs or updates siding, checks what it downloaded, and needs no `sudo`.
- The script is short enough to read before running it.

**Non-Goals:**
- Homebrew, signed or notarised binaries, Windows, `siding self-update`.
- Installing Docker or git, or running `siding setup`.
- Verifying signatures beyond the checksum from the same release.

## Decisions

### D1. POSIX `sh`, `curl` or `wget`
The script runs with `sh` on macOS and on minimal Linux images; it uses `curl -fsSL`, else
`wget -qO-`, and `sha256sum`, else `shasum -a 256`. Alternative: bash (not on every minimal
image, and `curl … | sh` is the common form).

### D2. The platform
`uname -s` gives `Darwin` or `Linux` (else the script stops), `uname -m` gives `arm64`/`aarch64`
→ `arm64` and `x86_64`/`amd64` → `amd64` (else it stops), naming what it found and what is
supported.

### D3. Download and check
Into a temporary directory removed on exit: the binary and `SHA256SUMS` from
`$SIDING_DOWNLOAD_URL`, which defaults to
`https://github.com/tweinmann/siding/releases/latest/download`, or to
`…/releases/download/$SIDING_VERSION` when that is set. The script compares the binary's sum with
its line in `SHA256SUMS` and stops on a mismatch or a missing line, before anything is
installed. `SIDING_DOWNLOAD_URL` also lets the test serve a local build, and lets a user point at
a mirror. Alternative: the GitHub API for the latest tag (rate-limited without a token, and needs
JSON parsing in `sh`).

### D4. Where it installs
`~/.local/bin` by default, created if missing: no `sudo`, and the directory the XDG convention
and many tools use. `SIDING_INSTALL_DIR` chooses another, e.g. `/usr/local/bin` with `sudo`.
The binary is moved into place with `mv` from the temporary directory in the same step as
`chmod 755`, so a running `siding` is replaced, not overwritten in place. When the directory is
not on the `PATH`, the script prints the `export PATH=…` line for `~/.zshrc` or `~/.bashrc`.
Alternative: `/usr/local/bin` by default (needs `sudo` on Linux and on many Macs).

### D5. Quarantine
`curl` and `wget` do not set macOS's quarantine attribute, so the script needs no `xattr`. The
manual way in the README keeps the `xattr` line for downloads through a browser.

### D6. The test
`hack/test-install.sh` (bash, the devcontainer guard of `hack/lib.sh`) builds with
`just dist v0.0.0-test` into a temporary directory, serves it with `python3 -m http.server` on a
free loopback port, and runs `install.sh` with `SIDING_DOWNLOAD_URL` and `SIDING_INSTALL_DIR`
pointing at temporary directories: an install that reports `v0.0.0-test`, an update over it, a
broken checksum that installs nothing, and an install directory missing from the `PATH`. It runs
in `just test`, needs no Docker and no network, and leaves nothing behind. The platform check is
tested with `uname` replaced by a function through `SIDING_UNAME_S`/`SIDING_UNAME_M`, which only
the test sets.

## Risks / Trade-offs

- [`curl | sh` runs code from the network] → the README shows the download-read-run variant; the
  script is short, and served from the repository over HTTPS.
- [The checksum comes from the same release as the binary] → it catches broken and truncated
  downloads, not a compromised release; signing is a non-goal for now.
- [`~/.local/bin` is not on the `PATH` on a fresh Mac] → the script says so and gives the line.
- [The first release does not exist yet] → until `v0.5.0` is tagged, `latest/download` answers
  404 and the script says that no release was found.

## Migration Plan

Nothing to migrate. The README changes with this change; the script works from the first
release, `v0.5.0`, which is tagged after this change is merged.
