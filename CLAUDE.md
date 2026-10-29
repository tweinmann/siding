# siding

siding gives every git worktree its own Docker Compose stack and one view of all of them. It
answers the question parallel coding agents leave open: does what my agent built actually run,
and what does it cost me right now? One static Go binary for macOS and Linux. Target users:
developers who run several coding agents or branches in parallel and whose app already runs with
Docker Compose.

OpenSpec is the only planning source: what siding does is in [openspec/specs/](openspec/specs),
the roadmap and the work in progress are the changes in [openspec/changes/](openspec/changes), and
the project context with principles, landscape and architecture is in
[openspec/config.yaml](openspec/config.yaml). Read the context and the relevant specs before
changing architecture.

## Status

Phase 0 approved (2026-10-28). Phase 1 (v0.1) and Phase 2 (v0.2) approved and archived
(2026-10-28, `openspec/changes/archive/2026-10-28-phase-1-stacks-and-routing` and
`…-phase-2-overview-and-budget`). Phase 3 (v0.3) approved and archived (2026-10-29,
`…/2026-10-29-phase-3-snapshots-and-queue`). Phase 4 (v0.4) approved and archived (2026-10-29,
`…/2026-10-29-phase-4-idle-and-wake`); its spike dropped Sablier (see its design.md). Phase 5
(v0.5) approved and archived (2026-10-29, `…/2026-10-29-phase-5-onboarding`); its release
`v0.5.0` is tagged from `main` once phases 3 to 5 are merged. siding today: `setup [--port]
[--budget] [--builds] [--idle]`, `teardown [--snapshots]`, `up [--wait] [--force] [--seed]`,
`status [--wait]`, `ps`, `logs`, `open`, `down [--volumes]`, `snapshot save|ls|rm`, `gc`,
`lint`, `ui`, `skill [--install] [--dir]`; routing by host and path through `siding-proxy`; a
memory budget with a start queue and learned peaks, where idle stacks make room (the verdict
`Stopped`); build slots; `EnvFileMissing` for env files of the main worktree; the snapshot
`default`; `just smoke-stacks`, `just build-darwin`, `just dist` and releases on `v*` tags.
Next: Phase 6 (`phase-6-remote-host`, v0.6), starting with `/opsx:propose`.

## Working agreements

- Work phase by phase, in the order of the `phase-<n>-<slug>` changes in `openspec/changes/`. At
  the end of each phase, stop, show the result, and wait for approval before starting the next.
- A phase is one OpenSpec change, `openspec/changes/phase-<n>-<slug>/`. It starts with
  `/opsx:propose`; the maintainer approves proposal, design, delta specs and tasks before
  `/opsx:apply`. A draft phase holds only its roadmap text and `skip_specs: true`; proposing it
  writes the delta specs and removes the marker. At the end of the phase, `/opsx:archive` merges
  the deltas into `openspec/specs/`, then stop for approval as above.
- `openspec/specs/` describes what siding does today. A change of behaviour changes its spec in
  the same change; a spec never describes a future phase.
- Ask about open design decisions instead of assuming. Record the answer in the change's
  `design.md`, with the alternatives. A decision that changes a convention, guardrail or safety
  rule also changes this file, or the context in `openspec/config.yaml`, in the same change.
- Keep this file, the README and the context current at the end of every phase.
- Everything in the repo is English: code, comments, docs, commit messages, CLI output.
  Conversation with the maintainer may be German or English.

## Guardrails

- Docker is the database. Everything siding knows comes from Compose labels and the Docker API.
  No database, no background service in v0.1. The only allowed state is a losable cache under
  `~/.siding`.
- The user's compose file is never edited. siding writes one override, `.siding/override.yml`,
  and runs `docker compose -p <repo>-<branch-slug> -f <compose file> -f .siding/override.yml …`.
- No published ports. The override resets them (`ports: !reset []`, Compose >= 2.24); HTTP
  services are reached through one shared Traefik, `siding-proxy`, on the network `siding`, by
  host (`<repo>.<branch-slug>.localhost`) and path (`dev.siding.route`).
- Runtime truth. A stack is Ready only when every service with a healthcheck is healthy and every
  routed service answers through the proxy. A failure names the service, the reason, a hint and
  the last log lines of that service.
- Agent and human see the same thing. Every reading command has `--json`; exit codes are
  0 Ready, 1 Failed, 2 TimedOut, 3 NoCapacity.
- Never lose work. Removing a stack's data (its volumes) or a worktree refuses when the branch
  is unmerged or the worktree is dirty, unless forced; removing containers and networks loses
  nothing and is not guarded (`siding down` keeps volumes, `down --volumes` is guarded). Every
  destructive command supports `--dry-run`.
- Only a human exposes anything to the internet. `siding share` is interactive only and has no
  `--yes`.
- Standard building blocks (Docker Compose, Traefik, later Sablier) over custom code.
- Removable. Uninstalling siding leaves plain Compose projects that keep running.
- Not an agent manager, not a sandbox for the agent, not Kubernetes, not production, not a
  hot-reload tool (Compose `develop.watch` is passed through).
- In a devcontainer, Docker-in-Docker only: no Docker-outside-of-Docker, neither for this
  repository's devcontainer nor as a supported setup for users, because the host's Docker socket
  is root on the host.

## Dev environment

All development happens inside the devcontainer. It runs its own Docker daemon
(Docker-in-Docker), which also hosts every stack siding's tests create. Nothing is installed on
the host. Tool versions, Compose included, are pinned in `.devcontainer/Dockerfile`, the Docker engine in
`.devcontainer/devcontainer.json`. The proxy port 8080 is forwarded to the host: run
`bin/siding setup --port 8080` once in the container, and `http://<host>.localhost:8080`
reaches a stack from the browser. The forward requires the host's 8080 (`requireLocalPort`):
when it is taken, VS Code says so instead of moving to another port. Free it, or change the
port in `devcontainer.json` and use the same one with `siding setup --port`.

CI runs `just test` on every push and `just smoke-docker` and `just smoke-stacks` on pull
requests and on `main`, all inside the devcontainer image.

| Command | Run from | Purpose |
|---|---|---|
| `just test` | devcontainer | level-1 checks, as in CI: gofmt, vet, `go test -race`, golangci-lint, `openspec validate --all --strict`, and `hack/test-install.sh` (`install.sh` against a release served on loopback) |
| `just golden` | devcontainer | rewrite golden files after an intended change |
| `just build` | devcontainer | build a static `bin/siding` (linux) |
| `just dist <version>` | devcontainer | build the release binaries (macOS and Linux, arm64 and amd64) and `SHA256SUMS` into `dist/`; CI does it on `v*` tags and publishes a GitHub Release |
| `just build-darwin` | devcontainer | build `bin/siding-darwin-arm64` and `-amd64`, to try siding on the Mac against Docker Desktop (product use, not development) |
| `just smoke-docker` | devcontainer | inner daemon: compose project with two services and healthchecks, `ports: !reset []`, Traefik on `siding` routing `demo.main.localhost` at `/` and `/api` |
| `just smoke-stacks` | devcontainer | inner daemon: siding's own commands end to end (`internal/e2e`, build tag `docker`): setup, `up --wait`, routing, a Failed verdict with log tail, `ps`, `down`, the guard of `down --volumes`; also checks golden overrides with `docker compose config` |
| `bin/siding setup --port 8080` | devcontainer | the real `siding-proxy` on the forwarded port, to try siding by hand |
| `openspec validate --all --strict` | devcontainer | check specs and active changes (part of `just test`) |
| `openspec list` / `openspec list --specs` / `openspec view` | devcontainer | active changes, specs, dashboard |
| `/opsx:explore`, `/opsx:propose`, `/opsx:apply`, `/opsx:archive` | Claude Code | the change workflow; skills in `.claude/skills/`, regenerated by `openspec update` |
| `hack/nuke.sh [--dry-run]` | host terminal (refuses to run in a container) | remove every Docker object siding development created on the host (only needs `docker`) |

## Safety rules

- All development and all tests run inside the devcontainer against its inner Docker daemon.
  Never mount the host's Docker socket or otherwise reach the host daemon from there. Scripts in
  `hack/` enforce this through `require_devcontainer`.
- The host Docker daemon is shared with the maintainer's other containers and volumes. The only
  siding objects on it are those in the footprint inventory of the spec `dev-environment`
  (devcontainer, its images and volumes). Delete them by exact name — never by prefix, never
  with `prune`. New host objects go into that spec, `hack/nuke.sh` and (for volumes)
  `devcontainer.json` together, in one change.
- Running siding on the host (e.g. `bin/siding-darwin-arm64` from `just build-darwin`) is use
  of the product, not development: what it creates there (`siding-proxy`, the network
  `siding`, the user's stacks, `~/.siding`) is not development footprint. `hack/nuke.sh` never
  removes it; siding's own commands and `docker compose -p siding down` do.
- Tests create stacks only as Compose projects with the prefix `siding-test-` and remove them by
  their exact name. Their snapshots are therefore named `siding-snap_siding-test-…` and are
  removed by exact name too; `just smoke-stacks` checks that none of either is left. The smoke tests' proxies listen on 18080, never on 8080; the end-to-end
  tests run their own proxy `siding-test-e2e-proxy` on the network `siding-test-e2e`, so a
  developer's `siding-proxy` is never touched.
- No real secrets in the repo, in golden files or in logs; golden files may contain obvious
  fakes such as `not-a-real-token`. `.env` values never appear in `--json` output.
- A worktree created with relative paths sets `extensions.relativeWorktrees` in `.git/config`,
  and Git before 2.48 then refuses to open the repository at all. The host's Git must be 2.48 or
  later before a worktree is created in the devcontainer.
- A plain `go build` in the devcontainer produces a Linux binary; a macOS build needs
  `GOOS=darwin`.

## Conventions

- Module `github.com/tweinmann/siding`, binary `siding`, API version `siding.dev/v1alpha1`.
- Labels on every container, network and volume siding creates or adopts: `dev.siding.repo`,
  `dev.siding.branch`, `dev.siding.worktree` (absolute path), `dev.siding.machine` (stable
  machine id); on containers also `dev.siding.gitdir` (the repository's common git directory)
  and `dev.siding.agent` (optional), never on networks or volumes, whose labels must not change
  between two `up`s. Service labels: `dev.siding.route` (`/`, `/<path>`, `false`),
  `dev.siding.port` (the HTTP port of a service with several), `dev.siding.seed`
  (`volume|script|none`), `dev.siding.seed.command` (the seed command of `script`). Volume
  label `dev.siding.shared`. Snapshot volumes: `dev.siding.snapshot` (name),
  `dev.siding.snapshot-volume` (the stack's volume key), `dev.siding.snapshot-created`, plus
  `dev.siding.repo` and `dev.siding.machine`. The snapshot named `default` seeds a stack's
  first `up` without `--seed`; `none` is not a snapshot name (`--seed none` starts empty). On `siding-proxy`: `dev.siding.budget` (the memory
  budget in bytes), `dev.siding.builds` (build slots), `dev.siding.idle` (the idle threshold in seconds, `0`
  for off) and `dev.siding.host-alias` (what `host.docker.internal` resolves to in the proxy).
- Compose project `<repo>-<branch-slug>`, host `<repo>.<branch-slug>.localhost`, proxy
  `siding-proxy` on the network `siding`, override `.siding/override.yml`.
- Docker state is read through the Docker Engine API (Go SDK); lifecycle operations invoke
  `docker compose`.
- What `--json` prints are document types of their own in `internal/cli/json.go`: `apiVersion`
  `siding.dev/v1alpha1`, a `kind`, empty fields left out rather than null, one golden file per
  kind in `internal/cli/testdata/json`. Within a version fields are only added. A failing command
  prints an `Error` document with a stable `code` and a `message` and exits 1.
- Go: standard layout (`cmd/`, `internal/`), table-driven tests, golden files in `testdata/`
  (`-update` via `just golden`). What a command needs from the machine goes into `cli.Options`;
  no package-level variable is a test seam, so tests run in parallel under `-race`.
- Shell: `hack/*.sh` use bash with `set -euo pipefail` and source `hack/lib.sh`;
  `hack/nuke.sh` is POSIX `sh` because it runs on the host; hack scripts require
  `SIDING_DEVCONTAINER=1`.
