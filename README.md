# siding

siding gives every git worktree its own Docker Compose stack, at its own `*.localhost` address,
and tells you and your coding agents whether it actually runs and what it costs.

> **Pre-alpha (v0.5).** Stacks per worktree, routing through one local proxy, the verdict of
> `siding status`, the overview `siding ui`, a memory budget with a queue, learned peaks and
> idle stacks that make room, snapshots and seeding, a build limit, `gc`, `lint` and a skill for
> coding agents work; expect
> rough edges and changes before v1. The plan is in [openspec/changes/](openspec/changes)
> (one change per phase), what siding does today in [openspec/specs/](openspec/specs).

## Get started

siding needs Docker with Compose 2.24 or later, and git. Your compose file stays as it is. The
steps below follow the order you walk them in; each links to the details in the
[reference](#reference).

### 1. Install siding

Download the binary for your platform and `SHA256SUMS` from the
[latest release](https://github.com/tweinmann/siding/releases/latest), then:

```sh
shasum -a 256 -c SHA256SUMS --ignore-missing      # sha256sum on Linux
chmod +x siding-darwin-arm64                       # siding-<os>-<arch> of your platform
sudo mv siding-darwin-arm64 /usr/local/bin/siding
xattr -d com.apple.quarantine /usr/local/bin/siding  # macOS: the binary is not signed yet
siding version
```

The binaries are static and need nothing but Docker and git. To build siding yourself, see
[Developing siding](#developing-siding).

### 2. Once per machine: the proxy

```sh
siding setup                 # Traefik as siding-proxy on 127.0.0.1:80
```

Every stack is then reached at `http://<repo>.<branch>.localhost`. Port taken, budget, build
slots: see [The proxy](#the-proxy) and [Memory budget and idle stacks](#memory-budget-and-idle-stacks).

### 3. Once per repository: prepare it

```sh
cd shop
siding lint                  # what breaks parallel stacks, before the first up
siding up --wait             # the stack of this worktree, and its verdict
```

Fix what [`lint`](#lint-rules) reports as errors in your compose file; siding never edits it.
Take its `LocalhostURL` warnings seriously: behind the proxy, `localhost` inside a container is
that container, so services call each other by name (`http://api:3000`), and the browser uses
paths on the stack's address (`/api`) instead of a fixed `http://localhost:3000`.
Label the service that is the app's root `dev.siding.route: "/"`, and give services
healthchecks, so that Ready means ready ([Routing](#routing)).

Teach your coding agent to use siding ([For coding agents](#for-coding-agents)):

```sh
siding skill --install       # .claude/skills/siding/SKILL.md; --dir for another harness
git add .claude/skills/siding && git commit -m "Add the siding skill"
```

Commit it: a new worktree only has committed files. `--install` also prints a section for
`AGENTS.md` and the permissions that let the agent run `siding up`, `status`, `logs` and `ps`
without asking; add them where your harness reads them.

To start every new worktree with data instead of empty, fill a stack with test data and save
it as the snapshot `default` ([Snapshots and seeding](#snapshots-and-seeding)):

```sh
siding snapshot save default # every new stack of this repository starts from it
```

### 4. Per task: a worktree and an agent

```sh
git worktree add ../shop-feature-x -b feature/x
cp .env ../shop-feature-x/   # git does not copy untracked files such as .env
cd ../shop-feature-x
```

If you forget, `siding up` refuses with `EnvFileMissing` and the `cp` command to run, instead
of starting the stack with blank values ([Env files](#env-files)).

Start your agent there. After a change, it runs `siding up --wait --json`, fixes what the
[verdict](#verdict-and-exit-codes) names, and reports the stack's address, e.g.
`http://shop.feature-x.localhost`, for you to try. Each worktree's stack has its own
containers, volumes, address and cookies.

### 5. Keep an eye on it

```sh
siding ps                    # every stack on this machine: state, address, memory
siding ui                    # the same as a page at http://siding.localhost
siding open                  # this worktree's stack in the browser
```

When a new stack does not fit into the memory budget, stacks nobody requested for a while make
room ([Memory budget and idle stacks](#memory-budget-and-idle-stacks)).

### 6. After the merge

```sh
siding gc                    # remove the stacks of merged branches; --dry-run first
git worktree remove ../shop-feature-x
```

See [gc](#gc) for what counts as merged and what stays.

## Reference

### The proxy

```sh
siding setup                 # Traefik as siding-proxy on 127.0.0.1:80
siding setup --port 8080     # if port 80 is taken, or inside siding's devcontainer
```

Every stack is reached through this one proxy at `http://<repo>.<branch>.localhost`. Browsers
resolve `*.localhost` to your machine by themselves, and so does curl on Linux. The curl of
macOS does not: there, use `curl -H "Host: shop.feature-login.localhost" http://127.0.0.1/`, or
add the host to `/etc/hosts`, as for other tools. Running `siding setup` again changes nothing
unless the port differs.

### Starting a stack

```sh
siding up                    # e.g. shop on feature/login: http://shop.feature-login.localhost/
siding up -- --build web     # arguments after -- go to docker compose up
siding up --wait             # and wait for the verdict, like siding status --wait
```

`up` runs your compose file as the Compose project `<repo>-<branch>` with one more file,
`.siding/override.yml`. That file resets every published port, so branches never fight over
ports, and puts siding's labels and routes on the services. `.siding/` ignores itself in git.

### Env files

`git worktree add` copies only what git tracks, so a new worktree has no `.env` and none of the
untracked files the compose file names in `env_file`. Compose would start the stack anyway,
with blank values. So in a linked worktree, `siding up` refuses with `EnvFileMissing` when it
lacks `.env` or a required `env_file` that the main worktree has and git does not track; the
hint gives the `cp` command, and `siding lint` warns `MissingEnvFile` for the same files.
siding never copies them itself: they hold secrets, and which values a branch needs is your
call. If a worktree needs none, an empty file will do (`touch .env`).

### Routing

- A service labelled `dev.siding.route: "/"` is the stack's root, `dev.siding.route: "/api"`
  answers under that path (the path is passed on unchanged), `dev.siding.route: "false"`
  hides a service.
- Every service with one container port (from `ports` or `expose`) also answers at
  `http://<service>.<repo>.<branch>.localhost`. With several ports, `dev.siding.port: "3000"`
  names the HTTP one.
- Published ports are gone, also a database's. Reach it with
  `docker compose -p <repo>-<branch> exec db psql …`.

### Verdict and exit codes

```sh
siding status                # Ready, Starting, Failed, TimedOut or Stopped, per service
siding status --wait         # until Ready, Failed or Stopped, at most --timeout (3m)
siding ps                    # every stack on this machine: state, address, memory
```

`Ready` means every service with a healthcheck is healthy and every service with a route
answers through the proxy. A failure names the service, the reason, a hint and its last log
lines. `Stopped` means no container runs and none failed; `siding up` starts it again. Exit
codes: 0 Ready, 1 Failed, 2 TimedOut, Stopped or still starting. Every command prints one
JSON document with `--json`, for scripts and agents.

### Lint rules

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
| `LocalhostURL` | warning | an environment value with `localhost:<port>` or `127.0.0.1:<port>`: behind the proxy it reaches the service's own container; the finding names the variable, never its value |
| `MissingEnvFile` | warning | in a new worktree, an untracked env file of the main worktree that is missing ([Env files](#env-files)) |
| `PublishedPort` | info | published ports, which siding resets |
| `NoRoot` | info | HTTP services, but none at `/` |

### Snapshots and seeding

```sh
siding snapshot save base    # copy this stack's named volumes into the snapshot "base"
siding snapshot ls           # the snapshots of this repository
siding snapshot rm base      # --dry-run lists it first
```

A snapshot is a copy of every named volume of a stack, kept as Docker volumes on this machine
per repository. While it copies, `save` stops the services that use those volumes, so a
database is copied in a consistent state, and starts them again. Snapshots outlive stacks:
`down --volumes` and `gc` never remove them; `siding teardown --snapshots` does.

```sh
siding up --seed base        # on the stack's first up: its volumes start as copies of "base"
siding up --seed none        # start empty, even with a snapshot default
```

The snapshot named `default` needs no flag: a stack's first `up` without `--seed` seeds from
it and says so on stderr, so an agent's first `up` in a new worktree gets the data without
knowing about snapshots. Save a newer state with `siding snapshot save default --force`; without
a snapshot `default`, a first `up` starts empty. `none` cannot be a snapshot's name.

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
  # without the label, or with dev.siding.seed: volume, the volumes come from --seed or default
```

The seed command runs with `docker compose exec` as soon as the service is ready. If it fails,
the verdict is `Failed` with the reason `SeedFailed` and the command's output.

### Memory budget and idle stacks

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

Before a stack waits or is refused, idle stacks make room for it. `up` stops other stacks that
had no request through the proxy for 5 minutes, the one idle longest first, as many as the new
stack needs, and only when that makes it fit; it says on stderr which ones it stopped. Stopping
removes nothing: containers stop, volumes and networks stay. A stopped stack reports the
verdict `Stopped` (exit code 2), and `siding up` in its worktree starts it again, in Compose's
order; a request to its address does not start it (the proxy answers 404).

```sh
siding setup --idle 15m          # another idle threshold; "default" is 5m
siding setup --idle off          # never stop a stack to make room
```

A request is anything that passes the proxy, `siding status` checks included. Work that does
not, such as tests through `docker compose exec` or a queue worker, does not count, so such a
stack can be stopped while it is busy; raise the threshold or turn it off if that matters. The
proxy records only the router and the time of each request, in its container log, which Docker
keeps at 30 MiB; no URL, header or cookie is kept. After upgrading siding, run `siding setup`
once, so the proxy records requests: until then, no stack is stopped to make room.

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

### gc

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

`siding skill` prints a skill, in the Agent Skills format, that tells an agent to run
`siding up --wait --json` after a change, to act on the verdict, and to report the stack's
address. It names no harness, so the same file works wherever skills are read.

```sh
siding skill --install                          # .claude/skills/siding/SKILL.md at the worktree's top
siding skill --install --dir ~/.claude/skills   # for every repository, nothing to commit
siding skill --install --dir <skills dir>       # the skills directory of another harness
siding skill > SKILL.md                         # print it, write nothing
```

`--install` replaces an older copy, so run it again after upgrading siding. It then prints two
snippets and writes neither: a short section for `AGENTS.md`, which many harnesses read, and
the permissions for siding's reading commands, as an entry for Claude Code's
`.claude/settings.json` and as a plain list for other harnesses. Only those four commands:
stopping and removing stacks and their data stays your decision.

### Removing siding

```sh
siding teardown              # remove the proxy; stacks keep running, --dry-run lists it
siding teardown --snapshots  # and every snapshot of this machine
```

Without siding, your stacks are plain Compose projects that keep running; `docker compose -p
<repo>-<branch> down` removes one. `~/.siding` is a cache you can delete.

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
   | `just dist <version>` | the release binaries and `SHA256SUMS` in `dist/`, as CI builds them for a `v*` tag |
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
