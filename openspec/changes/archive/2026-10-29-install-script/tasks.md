# Tasks

## 1. The script

- [x] 1.1 Write `install.sh` (D1–D5): platform, download with `curl` or `wget`, checksum, install to `~/.local/bin` or `SIDING_INSTALL_DIR`, `SIDING_VERSION`, the `PATH` hint, `siding version` and the next step, a clear message when no release is found; verify with `sh -n` and by the test in 1.2
- [x] 1.2 Write `hack/test-install.sh` (D6) and run it from `just test`: install, update, a broken checksum installing nothing, a directory missing from the `PATH`, an unsupported platform; verify that `just test` passes and leaves no temporary directory or server behind

## 2. Documentation

- [x] 2.1 Rewrite step 1 of the README's "Get started": the one-liner, the download-read-run variant, `SIDING_VERSION` and `SIDING_INSTALL_DIR`, the manual way with `xattr` for browser downloads, and `go install`; verify the commands against a `just dist` build served locally
- [x] 2.2 Add `install.sh` and `hack/test-install.sh` to CLAUDE.md's dev table where commands are listed, if they belong there; verify `openspec validate --all --strict` passes

## Workflow follow-up

- Stop and show the result; wait for the maintainer's approval.
- `/opsx:archive` merges the delta into `openspec/specs/`.
- After merging, tag `v0.5.0` and run the one-liner on the Mac against the real release.
