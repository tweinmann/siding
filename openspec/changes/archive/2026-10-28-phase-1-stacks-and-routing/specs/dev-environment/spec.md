# Spec Delta

## ADDED Requirements

### Requirement: Stack smoke test
`just smoke-stacks` SHALL run siding's own commands end to end against the inner daemon:
`setup`, `up --wait`, routing by host and path, a `Failed` verdict with log tail, `ps`, `down`
and the refusal of `down --volumes`. Its proxy, network and projects MUST carry the prefix
`siding-test-`, and its proxy MUST listen on 18080.

#### Scenario: Stack smoke test in CI
- **WHEN** a pull request is opened or pushed to, or a commit is pushed to `main`
- **THEN** CI runs `just smoke-stacks` inside the devcontainer, against its own daemon

#### Scenario: It leaves nothing behind
- **WHEN** `just smoke-stacks` ends, passing or failing
- **THEN** no container, network or volume whose name starts with `siding-test-` is left in the inner daemon, and `siding-proxy` and the network `siding` are untouched

#### Scenario: Not part of level 1
- **WHEN** `just test` runs without a Docker daemon
- **THEN** it passes, because the end-to-end tests only build with the `docker` build tag

## MODIFIED Requirements

### Requirement: Devcontainer settings
The devcontainer SHALL be named `siding-devcontainer`, set `SIDING_DEVCONTAINER=1`, forward port
8080 inside to 8080 on the host for the proxy and never to another host port, and set
`git config worktree.useRelativePaths true` when it is created, so worktrees work both inside
the container and on the host.

#### Scenario: A worktree made in the container
- **WHEN** `git worktree add` runs inside the devcontainer
- **THEN** the worktree's `.git` file and the repository's back-reference use relative paths

#### Scenario: The host's Git is older than 2.48
- **WHEN** a worktree with relative paths exists, and Git before 2.48 on the host opens the repository
- **THEN** it refuses with "unknown repository extension found: relativeworktrees", so the host needs Git 2.48 or later

#### Scenario: Port 8080 is taken on the host
- **WHEN** the devcontainer opens while another process holds port 8080 on the host
- **THEN** VS Code reports that it cannot forward 8080, instead of forwarding to another port that the addresses siding prints do not name
