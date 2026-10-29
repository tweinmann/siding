# Design

## Context

v0.3 (archived as `2026-10-29-phase-3-snapshots-and-queue`) has a memory budget that refuses or
queues a new stack, learned peaks, and no way to give back memory a stack holds but does not
use. The pieces this phase builds on: the proxy's compose file rendered by
`internal/proxy.ComposeFile` with settings as labels on `siding-proxy`; the override with one
router per route and a middleware `<project>-<service>-id` that adds `X-Siding-Service`;
`verdict.Evaluate` as a pure function; `checkBudget` and the queue loop in `up`; the losable
cache `~/.siding`. Traefik is pinned at v3.7.16. Motivation and scope: see proposal.md;
behaviour: see the delta specs. Constraints from CLAUDE.md: Docker is the database, no
background service, standard building blocks over custom code, never lose work.

Checked before this proposal (Sablier documentation and the plugin page on plugins.traefik.io):
Traefik v3.6 added the container label `traefik.docker.allownonrunning=true`, which keeps the
router and service of a stopped container, so a middleware still runs for its requests; the
Sablier plugin's middleware takes `group`, `sablierUrl`, `sessionDuration` and a strategy
(`dynamic` with a waiting page, or `blocking`); Sablier's Docker provider finds containers by
`sablier.enable` and `sablier.group`, stops them with `docker stop` once the session expired,
starts them on the next request, and treats a container as ready when it is healthy, or
running if it has no healthcheck. Not documented, and checked first in task 1: the order in
which Sablier starts the containers of a group, and whether a cached plugin loads offline.

## Goals / Non-Goals

**Goals:**
- Idle stop and wake built from Sablier and Traefik alone; siding only writes labels and the
  proxy's compose file, and reads Docker and one log file.
- A sleeping stack is visible as such (`Idle`, `idle`) and never woken by reading its state.
- Under pressure, memory comes from stacks nobody used for a while, never from active ones.
- Nothing changes for a user who does not run `setup --idle`.

**Non-Goals:**
- Idle stop per repository or per service, or excluding a service from it.
- Pausing instead of stopping (Sablier's `pause` strategy keeps the memory).
- Waking a stack for non-HTTP traffic (a database client through `docker compose exec`).
- Budget checks on a wake by request: the proxy cannot refuse a request for lack of memory.
- Showing the last request in `ps` or `ui`.

## Decisions

### D1. Sablier next to the proxy (maintainer's decision)
The project `siding` gains a service `sablier`, container `siding-sablier`, image
`sablierapp/sablier:1.10.1` pinned in the binary (the devcontainer's `SIDING_SABLIER_IMAGE`), on
the network `siding`, no published port, restarting unless stopped, with the Docker socket and
the flags `--provider.name=docker`, `--provider.auto-stop-on-startup=false` (a recreated
Sablier must not stop every stack at once) and `--storage.file=/data/sessions.json`, mounted
from `~/.siding/proxy/<project>/sablier/`, so sessions survive a recreation. `siding-proxy`
loads the plugin `github.com/sablierapp/sablier-traefik-plugin` at `v1.4.0` through
`--experimental.plugins.sablier.*`, with Traefik's plugin storage mounted from
`~/.siding/proxy/<project>/plugins/`. Traefik downloads the plugin when it starts; the cache
keeps later starts from needing the network (checked in task 1). Alternatives, asked: no wake
on request (`up` stops idle stacks under pressure, `siding up` wakes them; a request to a
sleeping stack would get 404); siding's own wake service (custom code where a standard building
block exists). Rejected: shipping the plugin's source as a local plugin (it would put Sablier's
code into siding's binary and repository).

The guardrail "no background service" becomes: siding itself runs no daemon; the proxy project
`siding` runs standard building blocks as containers (Traefik, Sablier). CLAUDE.md and
`openspec/config.yaml` change with this phase.

### D2. Opt-in with `setup --idle` (maintainer's decision)
`--idle <duration>` is parsed as a Go duration, at least 10 seconds (the end-to-end test uses
10s), and kept in seconds as the label `dev.siding.idle` on `siding-proxy`; `--idle off`
removes it; without the flag the label is kept, as with `--budget`. Sablier runs while the
label is set or any container carries `sablier.group` together with `dev.siding.repo` (Docker
is the database: setup asks the daemon, not a file). With `--idle off` and such containers
left, setup keeps Sablier and the plugin, so their routes keep working, and names the stacks
with the hint `siding up`; the next setup after they were started again removes Sablier.
Alternatives, asked: on by default with 30 minutes (would change v0.3 behaviour for every
user); a label per repository in the compose file (the user's file would have to change).

### D3. Labels in the override
When `up` finds `dev.siding.idle` on the proxy, the override adds to every service
`sablier.enable: "true"` and `sablier.group: <project>`, and to every routed service
`traefik.docker.allownonrunning: "true"` and a middleware `<project>-<service>-idle` with
`plugin.sablier.group=<project>`, `sablierUrl=http://<sablier container>:10000`,
`sessionDuration=<idle>s` and `blocking.timeout=2m`; its routers list `<name>-idle,<name>-id`.
Only container labels change, never those of networks or volumes. The idle duration is written
at `up`, so a changed `--idle` applies to a stack from its next `up`, as the spec says. One
group per Compose project means one request wakes the whole stack and any request keeps it
awake. Alternatives: one group per service (an `api` would sleep while `web` is used and fail
on its first call); the `dynamic` strategy (a waiting page answered by the plugin, not the
service: `curl` and the probe would read HTML that is not the app's, and agent and human would
see different things).

### D4. up registers the stack with Sablier
Sablier only stops what it has a session for. After `docker compose up -d`, `up` sends one
request through the proxy to the stack's first routed address with a 1-second timeout and
ignores the answer; the plugin opens the session before it waits for the stack. `up --wait`
does the same through its probes. A stack without a routed service has no address, gets no
session and never sleeps, which the spec states.

### D5. The verdict `Idle` (maintainer's decision: reported, never woken by status)
`verdict.Evaluate` gains the input "idle-enabled" (a container carries `sablier.group`) and
returns `Idle` before anything else when no container is running, restarting or starting and
every exit code is 0, 137 or 143. `status` evaluates the containers before it probes, and an
`Idle` stack is never probed, so reading never wakes it; `status --wait` ends on `Idle` like on
`Ready`. `Idle` exits 2 with the stack's `url` and the hint `a request to the url or siding up
--wait wakes it`. `ps` uses the same test for the state `idle`. A stack stopped with
`docker compose stop` while idle-enabled is `Idle` too, correctly so: a request wakes it.
Alternatives, asked: `status --wait` wakes the stack (a reading command with a side effect);
no state of its own (an agent would read `Failed` or `TimedOut` and start fixing code).

### D6. The last request: the proxy's access log
While Sablier runs, `siding-proxy` writes a JSON access log to
`~/.siding/proxy/<project>/access.log` with only the fields `StartUTC` and `RouterName`
(`--accesslog.fields.defaultmode=drop`, headers dropped), so no URL, header or client address
is kept. `internal/idle` reads the file backwards, at most 8 MiB, and maps router names
`<project>-<service>[-route]@docker` to the projects it is asked about, longest prefix first.
`up` rotates the file when it exceeds 16 MiB: it renames it to `access.log.1`, replacing the
old one, and sends `USR1` to `siding-proxy`, on which Traefik reopens its log; the reader also
looks into `access.log.1`. The file is a losable cache: a stack without a line in it has an
unknown last request and is never stopped to make room. siding's own probes count as requests,
on purpose: an agent polling `status` is using the stack. Alternatives: Sablier's sessions (its
API has no listing; its storage file is an internal format); Traefik's metrics (counters
without time; siding would have to sample them in the background).

### D7. Making room (maintainer's decision: only idle stacks, longest idle first)
Where `up` checks the budget (`checkBudget`, and in the queue only the first in line), a stack
that does not fit triggers: list the other stacks that run, are idle-enabled and whose last
request is known and at least 5 minutes old; sort them by last request, oldest first; take the
shortest prefix whose memory in use makes the new stack fit. If even all of them do not, stop
nothing. Otherwise stop each with `docker compose -p <project> stop` (by project name, no
compose file needed, as `down`), print `stopped idle stack <project> (no request for 20m,
1.0 GiB)` on stderr, read the memory again, and continue. `--force` skips this. The stopped
stacks are `Idle` and wake on their next request. The 5 minutes are a constant, not a setting.
Alternatives, asked: any other stack, least recently used first (could stop the stack another
agent is testing); none (only Sablier's duration frees memory).

### D8. Labels, codes, documents, tests
New label `dev.siding.idle` (proxy); the Sablier and `traefik.docker.allownonrunning` labels in
the override go into the conventions of CLAUDE.md and `openspec/config.yaml`. New verdict
`Idle`, new `ps` state `idle`, new error code `PluginUnavailable` (setup reads the proxy's log
after the start for Traefik's message that plugins are disabled). `Setup` gains `idleSeconds`.
`proxy.Config` gains the Sablier container name, so the end-to-end tests run
`siding-test-e2e-sablier` next to `siding-test-e2e-proxy`. Golden files: overrides with and
without idle, the proxy's compose file with and without Sablier, `Status` with `Idle`, `Setup`
with `idleSeconds`. The images are pulled in the inner daemon only; the host's footprint does
not change.

### D9. The skill
The skill names `Idle` under exit 2 with `siding up --wait --json` to wake the stack, says
that `siding status --json` holds the stack's `url` without starting anything, and that only
the addresses siding reports work, never `localhost:<port>` from the compose file. It does not
mention `--idle`: turning idle stop on or off is the user's decision.

## Risks / Trade-offs

- [Sablier starts a group's containers without Compose's `depends_on` order, so `api` may
  start before `db` is healthy] → task 1 checks it with a stack using
  `depends_on: condition: service_healthy`; if Sablier ignores the order, the phase stops there
  and the maintainer decides (wake through Compose instead, or accept with restart policies).
- [A one-off service (a migration that exited 0) is started again on wake] → task 1 checks
  what Sablier does with exited containers; migrations are usually idempotent; noted in the
  README.
- [The first start of the proxy with idle on needs the network to download the plugin] → the
  cache under `~/.siding/proxy/<project>/plugins/`; setup fails with `PluginUnavailable` and the
  hint `--idle off` instead of leaving routes without their middleware.
- [A wake by request ignores the budget] → `ps` and `ui` show the total over budget; the next
  `up` makes room or queues as usual. A non-goal of this phase.
- [The access log grows with traffic] → only two fields per line, rotated at 16 MiB.
- [Sablier holds the Docker socket and can start and stop any container] → it acts only on
  containers with `sablier.enable`, which siding writes only into its own override; it publishes
  no port and is reachable only on the network `siding`.
- [A long request blocks up to 2 minutes while a slow stack wakes] → the 503 after 2 minutes
  names nothing; `siding status` then shows which service is not ready.
- [Sablier or the plugin changes its labels or flags] → both pinned; the end-to-end test sleeps
  and wakes a stack against the pinned versions.

## Migration Plan

Nothing to migrate. A proxy of v0.3 has no `dev.siding.idle`; until `setup --idle` runs,
siding behaves as v0.3, and the proxy's compose file is byte for byte what v0.3 writes. Turning
idle stop on recreates the proxy project (stacks keep running) and applies to each stack from
its next `up`. Rollback: `siding setup --idle off`, then `siding up` in the worktrees setup
names, then `siding setup` once more removes Sablier.
