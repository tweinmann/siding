# Tasks

## 1. Foundations

- [x] 1.1 Add the error code `EnvFileMissing` and the lint rule names `MissingEnvFile` and `LocalhostURL`; verify with the error-code table test
- [x] 1.2 Add `MainTop` and `Tracked` to `internal/git` (D4); verify with tests on a real repository with a linked worktree, a bare-like custom layout, and a tracked and an untracked file
- [x] 1.3 Add reading the `env_file` paths from `docker compose config --no-env-resolution --format json` to `internal/compose`, with `required: false` kept; verify with a captured config and, in `just smoke-stacks`, against `docker compose config` with a missing file
- [x] 1.4 Restructure the README's "Using siding" as the user journey (D8): "Get started" in the order a user walks it (install, once per machine, once per repository, per task with a worktree and an agent, keeping an eye on it, after the merge), a few commands and sentences per step with links, and "Reference" with today's sections (routing labels, verdict and exit codes, lint rules, snapshots and seeding, memory budget and idle stacks, builds, devcontainer, removing siding); verify that no content of today's README is lost and every link resolves

## 2. Releases

- [x] 2.1 Add `just dist <version>` (D1): four static binaries in `dist/` with the version and `SHA256SUMS`; verify in the devcontainer that `dist/siding-linux-arm64 version` reports the version and `sha256sum -c SHA256SUMS` passes
- [x] 2.2 Add `.github/workflows/release.yml` on `v*.*.*` tags: `just test` and `just dist` in the devcontainer, then `gh release create --verify-tag --generate-notes` with `contents: write` for that job only; verify with `actionlint` if available, else by review, and by the first tag after approval
  - Checked by review and by parsing the YAML (actionlint was not available, its image was not pulled); the run itself is checked by the first tag after approval, see the workflow follow-up.
- [x] 2.3 Write step 1 of the README's journey, the install (download, `shasum -a 256 -c`, `chmod`, `PATH`, `xattr -d com.apple.quarantine` on macOS) and keep "build from source" for development; verify the commands against a `just dist` build in the devcontainer and on the Mac in task 7.2
  - Checked in the devcontainer against `just dist v0.5.0-test`; whether macOS's `shasum` knows `--ignore-missing` is checked on the Mac in 7.2.

## 3. The skill for any harness

- [x] 3.1 Implement `siding skill --install [--dir]` (D2): the default directory at the worktree's top, `~` expanded, an older copy replaced, `NotAGitRepository` outside a worktree without `--dir`, `--dir` without `--install` refused, `path` in `Skill`; verify with fake-based tests per scenario of `agent-skill` and the updated `skill.json` golden
- [x] 3.2 Print the `AGENTS.md` section and the permissions (D3), on stderr with `--json`; verify with a test that names the four reading commands and none of `down`, `gc`, `snapshot rm`, `teardown`, and that an existing `.claude/settings.json` is unchanged
- [x] 3.3 Keep the skill harness-neutral; verify with the skill test that it does not contain "Claude Code"
- [x] 3.4 Write the agent part of the journey's steps 3 and 4 and its reference: `--install`, committing the file, `--dir` for other harnesses and a global install, the snippets; verify the commands run as written in the devcontainer

## 4. Env files

- [x] 4.1 Check the env files of the main worktree in `up` before Compose runs (D4) and fail with `EnvFileMissing`, all missing files in the message and the `cp` commands and `touch .env` in the hint; verify with fake-based tests per scenario of `stack-lifecycle` "Env files of the main worktree" and an `error-envfilemissing.json` golden
- [x] 4.2 Report `MissingEnvFile` in `lint` and lint the rest on the `--no-env-resolution` config; verify with a fake-based test of a linked worktree without `.env` and one without an `env_file`
- [x] 4.3 Document env files in the journey's step 4 and the reference (what siding checks, why it never copies, `touch .env`); verify the example matches the tests

## 5. localhost in the environment

- [x] 5.1 Implement `LocalhostURL` in `internal/lint` (D5); verify with table tests for the spec's scenarios (an API URL, a password in the value that never reaches the finding, no port, `[::1]`) and the updated `lintreport.json` golden
- [x] 5.2 Add `LocalhostURL` and `MissingEnvFile` to the reference's lint table and explain `localhost` behind the proxy in the journey's step 3; verify the table matches the rule names in the code

## 6. The snapshot default

- [x] 6.1 Seed a first `up` without `--seed` from the snapshot `default` and plan none for `--seed none` (D6), with the stderr line; verify with fake-based tests per scenario of `seeding` "The default snapshot" and "Seed per service"
- [x] 6.2 Refuse `snapshot save none` and say what `default` means on `snapshot save default`; verify with fake-based tests
- [x] 6.3 Document the snapshot `default` in the journey's step 3 and the reference's snapshots and seeding; verify the shown commands run as written in the devcontainer

## 7. End-to-end and integration

- [x] 7.1 Extend `internal/e2e`: a linked worktree without `.env` refused with `EnvFileMissing`, then Ready after the hint's `cp`; a snapshot `default` seeding a new worktree's first `up`, and `--seed none` starting empty; `siding skill --install` writing into a temporary repository; verify with `just smoke-stacks` and that no `siding-test-*` object is left
- [x] 7.2 Walk the user journey on the Mac with a binary from `just dist`, following only the README's "Get started" (install, `setup`, `lint`, `skill --install`, a new worktree without `.env`, the snapshot `default`), and note every place where the README left a gap; record the result in this task
  - macOS with Docker Desktop (2026-10-29): passed without problems, following only the README's "Get started" with the `v0.5.0-rc1` binaries of `bin/mac-journey` and its demo project: install with `shasum -a 256 -c --ignore-missing`, `xattr` and `siding version`; `setup`; `lint` with `LocalhostURL` and `NoRoot`; `skill --install`; `snapshot save default`; a new worktree refused with `EnvFileMissing`, then seeded from `default`; `ps`, `ui`, `open`; `gc --dry-run`. The maintainer found no gap in the README.
- [x] 7.3 Update the status in CLAUDE.md, the README and `openspec/config.yaml` to v0.5, with `EnvFileMissing` in the conventions where error codes are listed; verify `just test` passes

## Workflow follow-up

- Stop and show the result; wait for the maintainer's approval of Phase 5.
- `/opsx:archive` merges the deltas into `openspec/specs/`.
- After approval, tag `v0.5.0` and check the published release.
