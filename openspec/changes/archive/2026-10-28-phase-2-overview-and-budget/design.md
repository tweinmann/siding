# Design

## Context

v0.1 (archived as `2026-10-28-phase-1-stacks-and-routing`) has every building block this phase
reuses: `ps` collects stacks and memory from the Docker API, `down` knows how to remove a stack
by project name and how to guard its volumes, `internal/stack` derives routes from the
normalised compose config, and `cli.Options` holds every seam to the machine. Motivation and
scope: see proposal.md; behaviour: see the delta specs. Constraints from CLAUDE.md that shape
this phase: no background service, Docker is the database (only a losable cache under
`~/.siding`), never lose work, the compose file is never edited.

## Goals / Non-Goals

**Goals:**
- One machine-wide view and one machine-wide limit, built on what `ps` already reads.
- `gc` and `down --volumes` share one notion of "merged", tested against real git repositories.
- Every new command fully testable with fakes; `just smoke-stacks` covers `ui`, the budget
  refusal and `gc` against the inner daemon.

**Non-Goals:**
- Actions in the web page (start, stop, gc): the page is read-only; actions stay in the CLI.
- A queue instead of refusing, learned memory peaks, a parallel build limit: v0.3.
- Removing git worktrees or branches.
- Docker-outside-of-Docker, in this repository's devcontainer or as a supported setup for
  siding inside a user's devcontainer.

## Decisions

### D1. `siding ui` in the foreground (maintainer's decision)
`siding ui` serves the page while it runs and removes its route when it stops. It keeps the
guardrail "no background service". Alternatives, asked: a container `siding-ui` next to the
proxy (always there, but a background service and a published Linux image of siding, since the
macOS binary cannot run in a container); a static HTML file (no server, but no live data and
no `siding.localhost`).

### D2. How the proxy reaches `siding ui`
Traefik's file provider watches `~/.siding/proxy/<project>/routes/`, mounted read-only into
`siding-proxy`. `siding ui` writes `ui.yml` there (atomically, via rename) with a router for
`Host(\`siding.localhost\`)` to `http://host.docker.internal:<port>`, and removes it on exit.
The address `host.docker.internal` depends on where the daemon runs, which `setup` decides
once:
- The gateway of the network `siding` is an address of this machine (Docker Engine on Linux,
  and the devcontainer's inner daemon): `extra_hosts: host.docker.internal:<gateway>`, and
  `siding ui` listens on that gateway and on `127.0.0.1`.
- Otherwise the daemon runs in a VM (Docker Desktop, OrbStack): `extra_hosts:
  host.docker.internal:host-gateway`, and `siding ui` listens on `127.0.0.1`, which these VMs
  forward to.

`setup` stores its choice as the label `dev.siding.host-alias` on `siding-proxy` (the gateway's
address, or `host-gateway`), so `siding ui` reads from Docker where to listen, and a proxy
without the label (from v0.1) is one that needs `siding setup`. Whether the gateway belongs to
this machine is a seam in `cli.Options` (`IsLocalAddress`).

Tried in the inner daemon before this proposal: `host-gateway` resolves to the default
bridge's gateway, not the proxy network's, and gave 502; the pinned gateway gave 200, and
removing the file gave 404 within two seconds. The VM case is tested by hand on the Mac
(task 9.3), which is what `just build-darwin` is for. Never listening on `0.0.0.0` keeps the
page off the LAN. Alternatives: a dummy container with Traefik labels that forwards to the
host (one more container, still needs the same address); the Traefik API (needs a second
published port).

### D3. The page
Server-rendered with `html/template`, refreshed by `<meta http-equiv="refresh" content="5">`,
no JavaScript and no assets outside the binary. The handlers call the same functions as
`ps --json` and `gc --dry-run --json`, so `/api/stacks` and `/api/gc` are those documents byte
for byte. Port: `--port`, else a free one from the kernel; `ui` prints both addresses.
Alternative: a JavaScript front end polling the JSON endpoints (nicer, but a build step and a
second code base for a read-only table).

### D4. Budget on the proxy (maintainer's decision)
The label `dev.siding.budget` (bytes) on `siding-proxy`, written into the proxy's compose file
by `setup --budget`; `setup` without `--budget` carries the running proxy's label forward;
`--budget default` drops it. Without a label the budget is 75 % of `docker info`'s `MemTotal`
(the VM's memory on Docker Desktop). Sizes parse as `<n>[k|m|g]` with binary units, or plain
bytes. Alternatives, asked: refuse only when the use already reached the budget (a new stack
may overshoot it by gigabytes); a configuration file under `~/.siding` (changes the guardrail
that `~/.siding` is only a losable cache). The new label goes into the conventions in CLAUDE.md
and `openspec/config.yaml`.

### D5. The estimate and the check
`up` checks after reading the config and before writing the override: use = memory of the
running containers of every siding stack except this project (the same stats `ps` reads),
estimate = per service and replica (`deploy.replicas`, `scale`), `deploy.resources.limits.memory`,
else `mem_limit`, else 256 MiB. Refuse when use + estimate > budget. The 256 MiB default is a
guess that v0.3 replaces with learned peaks. `--force` skips the refusal and warns.

### D6. setup recreates through Compose
Instead of comparing port and image itself, `setup` always rewrites the proxy's compose file
and runs `docker compose up -d --wait`; Compose recreates the container only when the file
differs. `setup` compares the container ID before and after to report `created`, `updated`
or `unchanged`. This covers the budget label and the new file provider, and a v0.1 proxy is
upgraded by the next `setup` on the port it had. Alternative: a label with a hash of the
configuration (one more label to keep in step with what Compose already compares).

### D7. "Merged" (maintainer's decision for gc's scope; the shared definition decided here)
`internal/git` gains one function for both commands: merged = the tip is an ancestor of the
default branch, or the upstream branch is gone (`%(upstream:track)` is `[gone]`, which a squash
merge with a deleted remote branch leaves after `git fetch --prune`). `gc` adds two conditions:
not the default branch itself, and the default branch has moved past the tip. Without them a
branch an agent just created, which has no commit of its own, would count as merged and its
running stack would be removed. `down --volumes` keeps v0.1's rule plus the squash case:
removing the data of a branch without own commits loses no work. Alternatives: only ancestry
(squash merges always need `--force`); the GitHub API (a network call and a token).

### D8. `gc` (maintainer's decision: stacks only, never worktrees)
A pure planning function takes the stacks of this machine (`dev.siding.machine` equals ours)
and git facts per stack and returns actions; the command fetches the facts, prints the plan,
asks, and removes with `docker compose -p <project> down --remove-orphans [--volumes]`. For a
stack whose worktree is gone, git runs as `git --git-dir=<dev.siding.gitdir>`. The label goes
on containers only: a new label on networks makes Compose recreate them (seen in Phase 1),
which would stop every v0.1 stack at its first v0.2 `up`; on containers it costs one recreate
of the containers, no data. Stacks without the label are treated as unmerged. Asking reads
`Options.Stdin` when `Options.IsTerminal()` is true; otherwise `--yes` or `--dry-run` is
required (`NotInteractive`). Alternatives, asked: also `git worktree remove` for merged, clean
worktrees; only stacks whose worktree is gone.

### D9. `lint`
The rules and severities are those of the spec `compose-lint`. Lint reads the same normalised
config as `up`, with the project siding would use (from git when the directory is a worktree,
else the directory's slug), so a fixed name is one that differs from `<project>_<key>`. The
worktree for `HostPath` is the git top level, else the compose file's directory. The parser in
`internal/compose` gains `container_name`, bind mounts, healthchecks, memory limits, replicas
and labels of volumes. Lint errors exit 1 with a `LintReport` (like a Failed verdict), not with
an `Error` document: the command worked, the file has problems. Alternatives: a rule set that
fixes things (would edit the user's compose file); only errors, no warnings (hides why a
verdict may be weaker than expected).

### D10. Devcontainers: Docker-in-Docker only (maintainer's decision)
This repository's devcontainer stays Docker-in-Docker, and siding inside any devcontainer is
supported with Docker-in-Docker only. Docker-outside-of-Docker gives code in the container the
host's daemon, which is root on the host, the opposite of why people put agents into
devcontainers; it also breaks bind mounts unless the workspace has the same path inside and
out, and needs siding to probe through `host.docker.internal`. Trying siding on real projects
happens on the Mac with `just build-darwin` (both architectures, `CGO_ENABLED=0`). What siding
creates there is product use, not development footprint: `hack/nuke.sh` leaves it alone.
Alternatives, discussed: a second devcontainer configuration with the host's socket, used only
to try siding while tests refuse it; tests against the host daemon with the prefix
`siding-test-`. The roadmap line and the architecture in `openspec/config.yaml` change with it;
the README gets a short "siding in a devcontainer" section with what Docker-in-Docker means
there (one view and one budget per container, one forwarded port per container, own image
cache).

### D11. Error codes and exit 3
`NoCapacity` is the first error that does not exit 1: `Execute` maps it to exit 3, with the
usual `Error` document. `NotInteractive` is a normal error (exit 1).

### D12. `siding teardown` (added during apply, at the maintainer's request)
The counterpart of `setup`: `docker compose -p siding down --remove-orphans` from the proxy's
directory, then that directory, then the network `siding` through the API, but only when no
container is attached to it. It never touches stacks; they keep running as plain Compose
projects (the guardrail "removable"), and teardown lists them with the hint `siding down` or
`siding gc`. `~/.siding/machine-id` stays, so stacks started later carry the same machine id.
Alternatives: refuse while stacks exist (forces removing work to uninstall the proxy); also
remove the stacks (would remove data and break "never lose work"); remove all of `~/.siding`
(changes the machine id of stacks that still run).

## Risks / Trade-offs

- [The VM case of D2 is untested in CI] → a manual test on the Mac in the tasks; the error
  when the page cannot be reached names `siding setup` and the address `siding ui` listens on.
- [Two `up`s at the same moment both pass the budget check] → accepted for v0.2; the queue of
  v0.3 serialises starts.
- [The 256 MiB default is wrong for big services] → users set `mem_limit`; `lint` could suggest
  it later; v0.3 learns peaks.
- [`setup --budget` recreates the proxy] → routing pauses for the seconds Traefik needs to
  start; stacks keep running (the spec `proxy` already requires it).
- [A killed `siding ui` leaves its route file] → the proxy answers 502 for `siding.localhost`;
  the next `ui` replaces the file.
- [`dev.siding.gitdir` recreates v0.1 containers once] → containers only, volumes stay; noted
  in the README's upgrade line.
- [A branch whose upstream was deleted without a merge counts as merged] → its worktree must be
  clean, so only committed work is affected, and it is still in the local branch, which gc never
  deletes.

## Migration Plan

- Run `siding setup` once after upgrading: it recreates the proxy with the file provider and
  host alias on its current port. `up`, `status` and the budget work with a v0.1 proxy;
  `siding ui` asks for `siding setup`.
- The first `up` of an existing stack recreates its containers once (new label); networks and
  volumes are untouched.
- Rollback: siding v0.1 works with a v0.2 proxy (it only reads the port).
