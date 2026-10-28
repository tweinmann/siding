# Proposal

## Why

siding exists to answer "does what my agent built actually run?". Today it only prints its
version. v0.1 is the smallest siding that answers the question: every worktree runs its own
stack from the unchanged compose file, every stack is reachable by name through one proxy, and
an agent can wait for a verdict, Ready or Failed with the reason and the logs, as versioned
JSON.

## What Changes

- `siding setup [--port <n>]`: starts the shared proxy `siding-proxy` (Traefik, Docker
  provider) on the network `siding`, listening on `127.0.0.1:80` by default. Idempotent.
- `siding up [--wait] [-- <compose up args>]`: in the current worktree, writes
  `.siding/override.yml` (ports reset, network `siding`, Traefik and `dev.siding.*` labels) and
  runs the user's compose file as project `<repo>-<branch-slug>`. With `--wait` it ends with the
  verdict of `status --wait`.
- `siding status [--wait [--timeout <d>]]`: the verdict Ready, Starting, Failed or TimedOut;
  a failure names the service, the reason, a hint and the last log lines of that service.
- `siding ps`: every stack on the machine, read from labels and the Docker API, with its
  state, address and memory use.
- `siding logs [service…]`, `siding open [service]`: the stack's logs and its address.
- `siding down [--volumes] [--force] [--dry-run]`: removes the stack's containers and network.
  Volumes go only with `--volumes`, which refuses when the branch is unmerged or the worktree
  dirty, unless `--force`.
- Routing: `<repo>.<branch-slug>.localhost` for the stack; `dev.siding.route` (`/`,
  `/<path>`, `false`) mounts services on it; every HTTP service also answers at
  `<service>.<repo>.<branch-slug>.localhost`. A new service label `dev.siding.port` picks the
  port of a service with several.
- `siding skill`: prints a Claude Code skill that tells an agent how to start its stack and
  wait for the verdict.
- Every command has `--json`, with the new kinds `Setup`, `Stack`, `Status`, `StackList`,
  `Logs`, `Address`, `Down` and `Skill`, and new stable error codes.
- The guardrail "never lose work" is made precise: it guards removing a stack's data (its
  volumes) or a worktree; removing containers loses nothing and is not guarded. CLAUDE.md and
  the project context change with it, as does the convention list (`dev.siding.port`).

## Capabilities

### New Capabilities
- `cli-contract`: the `--json` documents, the `Error` document, exit codes and `siding version`
  (exists since Phase 0, now specified).
- `stack-identity`: how a worktree maps to repo name, branch slug, Compose project, host names,
  machine id and the `dev.siding.*` labels.
- `proxy`: `siding setup` and the shared Traefik `siding-proxy` on the network `siding`.
- `routing`: which services are HTTP services, how route labels map to hosts and paths, and
  `siding open`.
- `stack-lifecycle`: `siding up`, `siding down`, `siding logs`, the override file and the
  guard that keeps unmerged or dirty work.
- `stack-status`: the verdict of `siding status`, its reasons, hints and log tail.
- `stack-list`: `siding ps`.
- `agent-skill`: the Claude Code skill printed by `siding skill`.

### Modified Capabilities
- `dev-environment`: a second smoke test, `just smoke-stacks`, runs siding's own commands
  end to end against the inner daemon, in CI on pull requests and on `main`.

## Impact

- Go code under `internal/` (git, compose, Docker, routing, verdict) and new commands in
  `internal/cli`; the Docker Engine Go SDK as a new dependency.
- `docker compose` (>= 2.24) and `git` become runtime requirements of siding.
- New golden files per document kind and for generated overrides; an end-to-end test under a
  `docker` build tag; `justfile` and CI gain `smoke-stacks`.
- CLAUDE.md, README and `openspec/config.yaml`: status, the precise guardrail, the label
  `dev.siding.port`, the proxy port.
