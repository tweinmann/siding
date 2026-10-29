# Proposal

## Why

Once siding runs, the core works: a stack per worktree, a verdict, an address. The first
attempt fails before and beside it. siding can only be built from source. The skill is missing
in a new worktree unless it was committed, and Claude Code asks before every `siding up`. A new
worktree has no `.env`, so Compose starts the stack with blank values, and its database is
empty. An app that calls `localhost:<port>` breaks behind the proxy without a warning. Each of
these is small; together they decide whether the first attempt works.

## What Changes

- **Releases.** `just dist` builds static binaries for macOS and Linux, arm64 and amd64, with
  the version from the tag, and `SHA256SUMS`. A workflow on `v*` tags publishes them as a
  GitHub Release. The README gets an install section, including macOS's quarantine flag.
  Homebrew and notarisation come later.
- **`siding skill --install [--dir <skills dir>]`** writes the skill as `<dir>/siding/SKILL.md`,
  by default into `.claude/skills` at the worktree's top. It says to commit the file, and prints
  two snippets: a section for `AGENTS.md`, which many harnesses read, and the permissions for
  siding's reading commands, as a Claude Code `settings.json` entry and as a plain list. siding
  writes no other file; without `--install`, `siding skill` prints the skill as before. The
  skill's text is harness-neutral.
- **Env files of the main worktree.** When a linked worktree lacks `.env`, or an `env_file` of
  the compose file, that the main worktree has as an untracked file, `up` fails with the new
  code `EnvFileMissing` and the `cp` command; `lint` warns `MissingEnvFile`. siding never
  copies them.
- **`lint` rule `LocalhostURL`**: a service's environment value pointing at `localhost` or
  `127.0.0.1` with a port, a warning that names the variable but never its value.
- **A default snapshot.** A snapshot named `default` seeds every first `up` of the repository
  that gives no `--seed`; `--seed none` starts empty. Without a snapshot `default`, nothing
  changes. `none` becomes a reserved name.

## Capabilities

### New Capabilities
- `releases`: how siding is built for distribution and published on a tag.

### Modified Capabilities
- `agent-skill`: `--install` with `--dir`, and the snippets for `AGENTS.md` and permissions.
- `stack-lifecycle`: `up` refuses when env files of the main worktree are missing.
- `compose-lint`: the warnings `MissingEnvFile` and `LocalhostURL`.
- `seeding`: the snapshot `default` and `--seed none`.
- `snapshots`: `none` is not a snapshot name.
- `cli-contract`: the error code `EnvFileMissing`.

## Impact

- Code: `internal/cli` (`skill`, `up`, `lint`, `snapshot save`), `internal/lint` (two rules),
  `internal/git` (the main worktree, tracked files), `internal/seed`, `internal/e2e`.
- Build: `justfile` (`dist`), `.github/workflows/release.yml`; `cmd/siding` already takes the
  version through `-ldflags`.
- Documents: `Skill` gains `path` when installed; `Error` gains the code `EnvFileMissing`. Only
  additions.
- README: install, the skill for any harness, env files, `localhost` behind the proxy, the
  snapshot `default`. CLAUDE.md and `openspec/config.yaml`: status and the new error code.
