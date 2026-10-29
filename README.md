# siding

siding gives every git worktree its own Docker Compose stack, at its own `*.localhost` address,
and tells you and your coding agents whether it actually runs and what it costs.

> **Pre-alpha (v0.3).** Stacks per worktree, routing through one local proxy, the verdict of
> `siding status`, the overview `siding ui`, a memory budget with a queue and learned peaks,
> snapshots and seeding, a build limit, `gc` and `lint` work; expect
> rough edges and changes before v1. There are no releases
> yet: build it from source (see below). The plan is in [openspec/changes/](openspec/changes)
> (one change per phase), what siding does today in [openspec/specs/](openspec/specs).

## Using siding

siding needs Docker with Compose 2.24 or later, and git. Your compose file stays as it is.

### Once per machine: the proxy

```sh
siding setup                 # Traefik as siding-proxy on 127.0.0.1:80
siding setup --port 8080     # if port 80 is taken, or inside siding's devcontainer
siding teardown              # remove the proxy again; stacks keep running, --dry-run lists it
```

Every stack is reached through this one proxy at `http://<repo>.<branch>.localhost`. Browsers
resolve `*.localhost` to your machine by themselves, and so does curl on Linux. The curl of
macOS does not: there, use `curl -H "Host: shop.feature-login.localhost" http://127.0.0.1/`, or
add the host to `/etc/hosts`, as for other tools. Running `siding setup` again changes nothing
unless the port differs.

### In a worktree: start the stack

```sh
siding up                    # e.g. shop on feature/login: http://shop.feature-login.localhost/
siding up -- --build web     # arguments after -- go to docker compose up
siding up --wait             # and wait for the verdict, like siding status --wait
```

`up` runs your compose file as the Compose project `<repo>-<branch>` with one more file,
`.siding/override.yml`. That file resets every published port, so branches never fight over
ports, and puts siding's labels and routes on the services. `.siding/` ignores itself in git.

- A service labelled `dev.siding.route: "/"` is the stack's root, `dev.siding.route: "/api"`
  answers under that path (the path is passed on unchanged), `dev.siding.route: "false"`
  hides a service.
- Every service with one container port (from `ports` or `expose`) also answers at
  `http://<service>.<repo>.<branch>.localhost`. With several ports, `dev.siding.port: "3000"`
  names the HTTP one.
- Published ports are gone, also a database's. Reach it with
  `docker compose -p <repo>-<branch> exec db psql …`.

### Does it run?

```sh
siding status                # Ready, Starting, Failed or TimedOut, per service
siding status --wait         # until Ready or Failed, at most --timeout (3m)
siding ps                    # every stack on this machine: state, address, memory
```

`Ready` means every service with a healthcheck is healthy and every service with a route
answers through the proxy. A failure names the service, the reason, a hint and its last log
lines. Exit codes: 0 Ready, 1 Failed, 2 TimedOut or still starting. Every command prints one
JSON document with `--json`, for scripts and agents.

### Before the first up: lint

```sh
siding lint                  # exits 1 when it finds an error; --json for a LintReport
```

`lint` checks the compose file siding would use and never changes it.

| Rule | Severity | Finds |
|---|---|---|
| `ContainerName` | error | a fixed `container_name`: a second stack cannot start |
| `FixedNetworkName` | error | an own network with a fixed `name`, shared by every stack |
| `FixedVolumeName` | error (info with `dev.siding.shared`) | an own volume with a fixed `name`, so every stack shares its data |
| `InvalidRoute`, `AmbiguousPort` | error | route labels `siding up` would refuse |
| `HostPath` | warning | a bind mount of a path outside the worktree, shared by every stack |
| `NetworkMode` | warning | `network_mode`: siding cannot route the service |
| `NoHealthcheck` | warning | no healthcheck and no route: Ready only means it started |
| `PublishedPort` | info | published ports, which siding resets |
| `NoRoot` | info | HTTP services, but none at `/` |

### Snapshots

```sh
siding snapshot save base    # copy this stack's named volumes into the snapshot "base"
siding snapshot ls           # the snapshots of this repository
siding snapshot rm base      # --dry-run lists it first
```

A snapshot is a copy of every named volume of a stack, kept as Docker volumes on this machine
per repository. While it copies, `save` stops the services that use those volumes, so a
database is copied in a consistent state, and starts them again. Snapshots outlive stacks:
`down --volumes` and `gc` never remove them; `siding teardown --snapshots` does.

### Seeding a new stack

```sh
siding up --seed base        # on the stack's first up: its volumes start as copies of "base"
```

A new worktree's stack can start with data instead of empty. On its first `up` (the project
has no containers and no volumes yet), `--seed <snapshot>` creates its volumes as copies of the
snapshot before Compose starts it; a later `up` never seeds again. Per service, the label
`dev.siding.seed` chooses:

```yaml
services:
  db:
    image: postgres:17
    labels:
      dev.siding.seed: script                       # run a command once on the first up
      dev.siding.seed.command: "psql -U app -f /seed/dev.sql"
  cache:
    image: redis:7
    labels:
      dev.siding.seed: none                         # never seeded, starts empty
  # without the label, or with dev.siding.seed: volume, the volumes come from --seed
```

The seed command runs with `docker compose exec` as soon as the service is ready. If it fails,
the verdict is `Failed` with the reason `SeedFailed` and the command's output.

### Memory budget

```sh
siding setup --budget 12g        # memory for all stacks together; "default" is 75 % of Docker's
siding up --force                # start even if the stack does not fit
```

Before `up` starts a stack, siding adds the memory the other stacks use now to what this
stack will need: per service the highest use siding has seen for it in this repository, plus a
quarter and at most its limit; else its `mem_limit` (or `deploy.resources.limits.memory`); else
256 MiB. siding learns those peaks whenever it reads memory (`ps`, `ui`, `up`, a Ready `status`)
and keeps them in `~/.siding/peaks.json`, a cache you can delete at any time. If that exceeds the budget, `up` refuses with exit code 3 and says
how much is missing. Setting `mem_limit` on big services makes the estimate right.

With `siding up --wait`, a stack that does not fit waits in line instead, until another stack
frees memory or `--timeout` (3 minutes) is up; then it exits 3. Stacks start in the order they
began to wait. `siding ps` and `siding ui` show the line. A waiting `up` that you stop with
Ctrl-C, or that is killed, leaves the line. `siding ps`
shows the total against the budget.

### Overview

```sh
siding ui                    # http://siding.localhost while it runs; Ctrl-C stops it
```

The page lists every stack with its address and memory, the total against the budget, and what
`siding gc` would remove. It refreshes itself and is read-only; `/api/stacks` and `/api/gc`
return the documents of `siding ps --json` and `siding gc --dry-run --json`. It listens on this
machine only, and nothing of it keeps running after Ctrl-C. After upgrading siding, run
`siding setup` once, so the proxy knows the route.

### Builds

```sh
siding setup --builds 2      # at most two stacks build images at once; "default" is half the CPUs
```

When a stack has services with `build:`, `up` takes one of these build slots while Compose
builds and starts it, and waits for a free one first (within `--timeout`). Stacks of images
only never wait. The slots are lock files under `~/.siding/builds`; a stopped or killed `up`
frees its slot at once.

### Logs, address, and taking it down

```sh
siding logs -f api           # docker compose logs of the stack
siding open                  # the stack's address in the browser; siding open api for a service
siding down                  # remove containers and networks; volumes and data stay
siding down --volumes        # also the data; refused while the branch is unmerged or the
                             # worktree dirty, unless --force; --dry-run lists it first
```

### After a merge: gc

```sh
siding gc --dry-run          # what can go, and what stays and why
siding gc                    # asks, then removes; --yes for scripts
```

`gc` removes the stacks of merged branches whose worktree is clean, with their volumes, and
the containers of stacks whose worktree you deleted (their volumes only if the branch is
merged). A branch counts as merged when the default branch has moved past it, or when its
upstream branch is gone, as after a squash-merged pull request and `git fetch --prune`. `gc`
never deletes worktrees or branches, and never the stack of the default branch or of a branch
without commits of its own.

### In a devcontainer

siding works inside a devcontainer that runs its own Docker daemon (Docker-in-Docker), as this
repository's does. Then:

- `siding ps`, `siding ui` and the budget see the stacks of that container, not the whole
  machine; for one view of everything, run siding on the machine itself;
- forward the proxy's port (`forwardPorts` with `requireLocalPort`) and run
  `siding setup --port` with it; each devcontainer needs its own port;
- the container has its own image cache.

Mounting the host's Docker socket into the devcontainer (Docker-outside-of-Docker) is not
supported: it gives everything in the container root on the host, the opposite of why agents
run in devcontainers, and bind mounts break unless the paths match inside and out.

### For coding agents

`siding skill` prints a Claude Code skill that tells an agent to run `siding up --wait --json`
after a change and to act on the verdict. siding writes no file itself; install it with

```sh
mkdir -p .claude/skills/siding && siding skill > .claude/skills/siding/SKILL.md
```

## Developing siding

Everything runs inside the devcontainer, which has its own Docker daemon; nothing is installed
on your machine and siding's tests never touch your Docker containers.

1. Open the repository in VS Code and choose **Dev Containers: Reopen in Container** (needs
   Docker Desktop, OrbStack or Docker Engine).
2. In the container's terminal:

   | Command | Purpose |
   |---|---|
   | `just test` | formatting, vet, unit tests with `-race`, golangci-lint, OpenSpec validation (as in CI) |
   | `just golden` | rewrite golden files after an intended output change |
   | `just build` | build `bin/siding` |
   | `just build-darwin` | build `bin/siding-darwin-arm64` and `-amd64`, to run siding on your Mac |
   | `just smoke-docker` | Compose, `ports: !reset []` and Traefik routing against the container's own daemon |
   | `just smoke-stacks` | siding's own commands end to end against the container's own daemon |

   To try siding by hand: `just build`, then `bin/siding setup --port 8080` (the port the
   devcontainer forwards), and `bin/siding up --wait` in a project; its address,
   `http://<repo>.<branch>.localhost:8080`, opens in the browser on your machine. Port 8080
   must be free on your machine: VS Code reports it when it is not, rather than forwarding to
   another port. Then free it, or change the port in `.devcontainer/devcontainer.json` and pass
   the same one to `siding setup --port`.

To remove everything siding development left on your Docker host, run `hack/nuke.sh` in a
terminal on the host (`--dry-run` lists it first).

Working agreements, guardrails and safety rules are in [CLAUDE.md](CLAUDE.md).

## License

Apache-2.0, see [LICENSE](LICENSE).
