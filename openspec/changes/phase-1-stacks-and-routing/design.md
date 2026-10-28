# Design

## Context

Phase 0 left a Cobra root with `version`, the document types in `internal/cli/json.go`, an
`Error` type with a stable code, golden tests, and a shell smoke test showing that the building
blocks work: `ports: !reset []`, Traefik with the Docker provider on the network `siding`, Host
and PathPrefix rules (`hack/smoke/testdata/override.yml` is the hand-written model of what
`siding up` must generate). Motivation and scope: see proposal.md; behaviour: see the delta
specs. Constraints from CLAUDE.md: Docker is the database, the compose file is never edited, no
published ports, runtime truth, the same output for agent and human, no package-level test
seams.

## Goals / Non-Goals

**Goals:**
- Every package that touches the machine (git, `docker compose`, Docker API, HTTP probe, clock,
  browser) sits behind a small interface in `cli.Options`, so all commands, verdicts and
  overrides are tested without Docker, in parallel, against golden files.
- One end-to-end test of the real thing against the inner daemon (`just smoke-stacks`).

**Non-Goals:**
- Acting on a stack other than the current worktree's (`down <stack>`, `gc`): v0.2.
- Memory budget, exit 3, web overview: v0.2. Snapshots, seeds: v0.3. Idle stop: v0.4.
- Reaching a published database port from the host: ports are reset; `docker compose exec`
  or a routed admin UI is the way in v0.1.
- Path stripping, TLS, WebSocket specifics: Traefik passes WebSockets through as it is.

## Decisions

### D1. Package layout
`internal/git` (worktree, branch, common dir, dirty, merged), `internal/compose` (run
`docker compose`, parse `config --format json`), `internal/docker` (Engine API: containers,
networks, volumes, logs, stats), `internal/stack` (identity, slugs, labels, override
generation, routing table: pure functions), `internal/verdict` (from service states and probe
results to a verdict: pure), `internal/proxy` (the embedded proxy compose file, setup), and
`internal/cli` for commands and documents. Pure packages get table-driven tests; the others are
thin and covered by the end-to-end test.

### D2. Reading the user's compose file through `docker compose config`
siding runs `docker compose -f <file> config --format json` and reads services, ports,
`expose`, labels, networks, `network_mode`, healthchecks, volumes and `env_file` from the
normalised result. Compose does interpolation, profiles, `include` and `extends`; siding parses
none of it itself. Alternative: the compose-go library in process. Lost: it would pin a second
Compose implementation into the binary that can disagree with the user's `docker compose`
(principle 8, standard building blocks).

### D3. Docker Engine API through `github.com/moby/moby/client`
State (containers by label, health, exit codes, published ports, logs, one-shot stats) is read
through the Engine API. `github.com/docker/docker` is frozen at v28; the client now lives in
its own module. Lifecycle (`up`, `down`, the proxy) goes through `docker compose`, as the
conventions say. Alternative: parsing `docker inspect` output. Lost: slower, and more text
formats to keep stable.

### D4. Proxy as Compose project `siding` (maintainer's decision: port 80, `--port`)
`siding setup` writes the embedded compose file to `~/.siding/proxy/compose.yml` (a losable
cache, rewritten on every setup) and runs it as project `siding`, container `siding-proxy`,
image pinned in the binary (`traefik:v3.7.16`, the same as the devcontainer's
`SIDING_TRAEFIK_IMAGE`), published on `127.0.0.1:<port>`, port 80 by default. Traefik flags:
Docker provider, `exposedbydefault=false`, `network=siding`, and a constraint
`LabelRegex(\`dev.siding.repo\`, \`.+\`)`, so foreign Traefik labels are ignored. The port is
read back from the container's port binding (Docker is the database); no file holds it.
In the devcontainer the developer runs `siding setup --port 8080`, the forwarded port.

Alternatives for the port, asked: 8080 by default (every URL carries `:8080`); 80 with an
automatic fallback to 8080 (convenient, less predictable). Alternative for the mechanism:
a plain container created through the API. Lost: the proxy then is not a Compose project,
and "removable, plain Compose" (principle 9) would not hold for it.

### D5. Identity (maintainer's decision: repository name from the main worktree)
Repository name = basename of the parent of `git rev-parse --path-format=absolute
--git-common-dir`, overridable with `--repo`. Slugs as in the spec `stack-identity`; 40
characters keep `<service>.<repo>.<branch>.localhost` labels well under DNS's 63 and project
names short. Alternative, asked: the name of the `origin` remote (stable when the folder is
renamed, but needs a remote and differs from what the user sees in the file system).

A detached HEAD is an error (`NoBranch`), because a slug from a commit would make a new stack
for every commit. Collisions (`feature/a`, `feature-a`) are detected through the labels of the
existing project, not prevented by hashing every slug, so normal names stay readable.

### D6. Machine id
`sha256("siding-machine-id:" + os id)`, first 16 hex digits; the os id is `/etc/machine-id`
on Linux and `IOPlatformUUID` from `ioreg` on macOS. Without one (many containers), a random id
in `~/.siding/machine-id`. Hashing keeps the raw id, which other software treats as
confidential, out of labels. Losing the file changes the id; in v0.1 that only changes a label
nobody filters on yet, and v0.5 can make the fallback durable if remote hosts need it.
Alternative: always a generated file. Lost: reinstalling or clearing `~/.siding` would change
the id even where the OS has a stable one.

### D7. The override (maintainer's decision: `dev.siding.port` for ambiguous ports)
Generated from the normalised config, written as YAML with `!reset` tags:
- every service: `ports: !reset []`, labels `dev.siding.*`;
- every HTTP service not hidden: `networks:` = its normalised networks plus `siding` (a list
  in an override replaces the default network otherwise), `traefik.enable=true`,
  `traefik.docker.network=siding`, one router `<project>-<service>` for
  `Host(\`<service>.<repo>.<branch>.localhost\`)`, and for a route path a second router
  `<project>-<service>-route` for `Host(\`<repo>.<branch>.localhost\`)` plus
  `PathPrefix(\`<path>\`)` (none for `/`); Traefik's default priority (rule length) makes the
  longest prefix win;
- a headers middleware on every siding router that adds `X-Siding-Service: <project>/<service>`
  to responses (D9);
- top-level: labels on the project's own networks (`default` included) and named volumes, not
  on external ones; the network `siding` as external.
Golden files under `internal/stack/testdata/override/` hold input config and expected override.

HTTP detection, asked: exactly one container port from `ports` targets and `expose`, or
`dev.siding.port`. Alternatives: the first port wins (a database with a published port would
be routed by accident); only services with a route label (drops the service host of the
roadmap). The new label changes the convention list in CLAUDE.md and `openspec/config.yaml`.

A path prefix is passed on unchanged (no StripPrefix): the common dev setups (Vite, nginx,
Next.js rewrites) mount an API under `/api` and expect `/api` to arrive. A stripping option
can be added later as a label value without breaking this one.

### D8. `.siding/.gitignore`
siding writes `.siding/.gitignore` with `*`, which ignores the directory including itself, so
`.siding/` never makes the worktree dirty and the user's `.gitignore` is never touched.
Alternative: ask users to add `.siding/` to `.gitignore`. Lost: one more setup step, and a
forgotten one would make `down --volumes` refuse with `DirtyWorktree`.

### D9. Readiness probe through the proxy
siding sends `GET <route>` with `Host: <stack host>` to `127.0.0.1:<proxy port>` (no DNS
needed) for every service with a route path, and for `/` routes to `/`. A response counts as
the service's own when it carries `X-Siding-Service` for that service and its status is not
502, 503 or 504. Traefik's own 404 for a missing router has no such header; its 502/503/504
for an unreachable backend do not count. The probe follows no redirects; a 3xx counts.
Alternatives: any status below 500 (cannot tell Traefik's 404 from the app's); asking
Traefik's API whether the router exists (needs a second published port). The end-to-end test
checks that Traefik really adds the header to backend responses and not to its own errors;
if it adds it to its 502 too, the status check still rules those out.

### D10. Verdict
A pure function from (services in the config, containers with state, health, exit code,
restart count, probe results, elapsed time, timeout) to the verdict. Order of evaluation:
`Missing` (no container after `up` has finished), `Exited` (non-zero), `Unhealthy`,
`Restarting` (restart count grew by 3 or more while waiting, or state `restarting` at the end),
then `Starting` until healthy, running or exited 0 and answering. `--wait` polls every second.
A service that exits 0 counts as done (migrations, seeds). The first failing service in
compose order is reported; others appear in `services` with their state. The hint is a fixed
text per reason, completed with the exit code or the last healthcheck output. Log tail: 20
lines from the Docker API, redacted (D12).

`status` without `--wait` and with `Starting` exits 2: "not ready now" is the same answer as a
wait of zero. Alternative: exit 0 with the verdict in the document. Lost: a script checking
only the exit code would take Starting for Ready.

### D11. `down` (maintainer's decision: containers go, volumes stay)
`docker compose -p <project> down --remove-orphans` (by name, works without the compose file);
`--volumes` adds `-v` after the guard: `git status --porcelain` empty, and the branch tip an
ancestor of the default branch (`refs/remotes/origin/HEAD`, else local `main`, else
`master`, else unmerged). `--dry-run` lists containers, networks and, with `--volumes`,
volumes by label through the API. Alternatives, asked: `down` always removes volumes and is
always guarded (an agent could not stop its stack in a dirty worktree without `--force`);
`down` only stops (removing would wait for `gc`). This makes the guardrail "never lose work"
precise: it guards a stack's data and worktrees, not containers. CLAUDE.md and
`openspec/config.yaml` change with it. A squash-merged branch counts as unmerged; `--force`
is the way until `gc` (v0.2) learns more.

### D12. Redaction
Values from the worktree's `.env` and from every `env_file` in the normalised config, 4
characters or longer, longest first, are replaced by `***` in every log line siding puts into a
document. The `.env` is parsed with Compose's rules for `KEY=VALUE`, quotes and comments; no
values are logged or cached. Shorter values are left, since `***` for `1` or `on` would wreck
every line. Text output of `siding logs` stays raw: it is the user's own terminal, as with
`docker compose logs`.

### D13. `siding skill` (decided here, open for review)
The skill text is embedded in the binary (`internal/cli/skill/SKILL.md`, golden-tested) and
printed; siding writes no file into the user's repository. Alternatives: `siding skill
install` writing `.claude/skills/siding/SKILL.md` (one more file siding writes into a user's
repository); a Claude Code plugin marketplace entry (an extra channel to keep in step with the
binary). Printing keeps the text and the binary in step and lets the user choose where it goes.

### D14. Test seams
`cli.Options` gains `Git`, `Compose`, `Docker`, `Probe`, `Browser`, `Clock`, `Home`, `Env`
and `Proxy` (container name, network, project, image, default port). `main` fills them with
real implementations; unit tests use fakes. The end-to-end tests under `internal/e2e` with
`//go:build docker` run commands in process with `Proxy` set to `siding-test-proxy` on the
network `siding-test` at port 18080, in temporary git repositories whose `--repo` starts with
`siding-test-`, and remove everything by exact name in `t.Cleanup`. So the binary has no hidden
test switches, and a developer's `siding-proxy` in the devcontainer is never touched.

## Risks / Trade-offs

- [Some tools do not resolve `*.localhost`] → browsers and curl do; siding's own probe uses
  `127.0.0.1` with a Host header; README notes `curl` and `/etc/hosts` for others.
- [Port 80 is taken or not bindable] → `PortInUse` with the hint `--port 8080`; addresses then
  carry the port.
- [Users lose host access to published database ports] → documented in README with
  `docker compose exec`; a routed TCP option is out of scope for v0.1.
- [Traefik adds `X-Siding-Service` to its own errors] → the status check excludes 502/503/504;
  the end-to-end test pins the behaviour.
- [`docker compose config` differs between Compose versions] → siding requires 2.24 and checks
  the version once per command that runs Compose; golden tests cover the parsed fields.
- [Squash merges count as unmerged] → `--force`, and the hint says so.
- [Redaction misses secrets that are not in `.env` files, e.g. inline `environment:` values]
  → the guarantee is stated as `.env` and `env_file` values only, which is what the safety
  rule asks; inline values are visible in the compose file anyway.

## Migration Plan

Nothing to migrate: v0.1 is the first release with stacks. Uninstalling leaves plain Compose
projects (`<repo>-<branch>` and `siding`) that keep running and are removed with
`docker compose -p <name> down`.
