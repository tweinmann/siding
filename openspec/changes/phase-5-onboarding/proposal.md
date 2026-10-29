# Proposal

> Draft. This change holds the roadmap text only and is marked `skip_specs: true`.
> `/opsx:propose` turns it into a full change: it writes the delta specs, `design.md` and
> `tasks.md`, revises this proposal, and removes `skip_specs` from `.openspec.yaml`.

## Why

The core works once siding is set up, but the first attempt fails before and beside it: siding
can only be built from source, the skill is missing in new worktrees, Claude Code asks for every
`siding up`, a new worktree has no `.env` and an empty database, and apps that call
`localhost:<port>` break behind the proxy without a warning.

## What Changes

Roadmap v0.5 (decisions agreed with the maintainer on 2026-10-29):

- Releases: unsigned binaries for macOS and Linux (arm64, amd64) on GitHub Releases, built by CI
  on every tag; an install note for macOS's quarantine flag. Homebrew and notarisation later.
- `siding skill --install` writes `.claude/skills/siding/SKILL.md` and says to commit it;
  without the flag the skill is only printed, as today. siding prints the permission snippet for
  `.claude/settings.json` and never writes that file.
- A missing `.env` that the compose file uses: `up` fails with `EnvFileMissing` and the `cp`
  command from the main worktree, `lint` warns; siding never copies it.
- `lint` rule `LocalhostURL`: an environment value pointing at `localhost:<port>`.
- A default snapshot: `siding snapshot save <name> --default`; the first `up` of a stack seeds
  from it unless `--seed none`; without a default, nothing changes.

## Capabilities

To be decided at `/opsx:propose`.

## Impact

To be decided at `/opsx:propose`.
