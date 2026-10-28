# Proposal

## Why

v0.1 answers "does my branch run?" for one stack at a time. With several agents in parallel the
human needs the whole picture: what runs, what it costs, what can go after a merge. And the
machine needs a limit, so that the fifth stack does not make the first four unusable.

## What Changes

- `siding ui`: a read-only overview at `http://siding.localhost`, served by siding in the
  foreground while the command runs (no background service). It shows every stack with its
  state, address and memory, the budget, and what `gc` would remove.
- Memory budget: `siding setup --budget <size>` stores the budget as a label on
  `siding-proxy` (default: 75 % of Docker's memory). `siding up` refuses with exit 3
  (`NoCapacity`) when the other stacks' memory plus the new stack's estimate exceed it;
  `--force` starts it anyway. `siding ps` shows the budget.
- `siding gc [--dry-run] [--yes]`: removes stacks that can go: whose branch is merged and
  worktree clean (containers, networks, volumes), and whose worktree is gone (containers and
  networks; volumes only when the branch is merged). It never touches git worktrees or
  unmerged or dirty work, and asks before removing unless `--yes`.
- `siding lint`: checks a compose file for what breaks parallel stacks (fixed container,
  network or volume names, shared host paths, ambiguous ports, missing healthchecks), as a
  `LintReport` with error, warning and info findings.
- A new label `dev.siding.gitdir` (the repository's common git directory) on every stack, so
  `gc` can tell whether the branch of a deleted worktree is merged.
- Development: `just build-darwin` builds a macOS binary to try siding on the Mac against
  Docker Desktop. Docker-outside-of-Docker is dropped from the roadmap: the devcontainer stays
  Docker-in-Docker only, and siding inside any devcontainer is supported with
  Docker-in-Docker only.

## Capabilities

### New Capabilities
- `overview`: `siding ui`, the web page at `siding.localhost` and its JSON endpoints.
- `memory-budget`: the budget, how a new stack is estimated, and the refusal with exit 3.
- `stack-gc`: `siding gc` and what may and may not be removed.
- `compose-lint`: `siding lint` and its rules.

### Modified Capabilities
- `cli-contract`: exit 3 for `NoCapacity` with an `Error` document; new error codes
  `NoCapacity` and `NotInteractive`.
- `proxy`: `setup --budget`, and the proxy's route to `siding ui`.
- `stack-identity`: the label `dev.siding.gitdir`.
- `stack-list`: `ps` shows the budget.
- `stack-lifecycle`: `down --volumes` accepts a squash-merged branch (upstream deleted), as `gc` does.
- `agent-skill`: the skill explains exit 3.
- `dev-environment`: `just build-darwin`, siding used on the host is not development
  footprint, Docker-in-Docker only.

## Impact

- New commands `ui`, `gc` and `lint` in `internal/cli`; packages for the budget, gc planning,
  lint rules and the web page (Go `net/http`, `html/template`, no JavaScript framework).
- `siding-proxy` gains Traefik's file provider and a host alias; existing proxies are
  recreated by the next `siding setup`.
- `cli.Options` gains stdin and a terminal check (for `gc`'s question) and a listener.
- CLAUDE.md and `openspec/config.yaml`: the new label, the budget label, the roadmap without
  Docker-outside-of-Docker, devcontainers with Docker-in-Docker only.
