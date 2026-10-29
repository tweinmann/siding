# Design

## Context

v0.3 (archived as `2026-10-29-phase-3-snapshots-and-queue`) has a memory budget that refuses or
queues a new stack, learned peaks, and no way to give back memory a stack holds but does not
use. The pieces this phase builds on: the proxy's compose file rendered by
`internal/proxy.ComposeFile` with settings as labels on `siding-proxy`, recreated by Compose
when the file differs; router names `<project>-<service-slug>[-route]`; `verdict.Evaluate` as a
pure function; `checkBudget` and the queue loop in `up`. Traefik is pinned at v3.7.16.
Motivation and scope: see proposal.md; behaviour: see the delta specs. Constraints from
CLAUDE.md: Docker is the database, no background service, standard building blocks over custom
code, never lose work.

### Spike result (task 1.1, 2026-10-29)
The first version of this change planned idle stop and wake on request with Sablier next to the
proxy. The spike ran it in the inner daemon with Traefik v3.7.16, plugin v1.4.0, Sablier 1.10.1,
a stack of `db` (healthcheck, ready after 3 s), `migrate` (exits 0) and `web` (routed,
`depends_on: db: condition: service_healthy`), session 15 s, `blocking` strategy:

- Worked: Traefik keeps the route of a stopped container with `allownonrunning`; a request
  opens a session; Sablier stops the group after the session (exit codes 137 and 0); a request
  wakes it; an access log with only `RouterName` and `StartUTC`; `USR1` reopens a renamed log.
- **Start order not kept**: on wake, `db` and `web` started in the same second; `web` did not
  wait for `db` to be healthy. Compose's `depends_on` is ignored.
- **One-off services run again on every request**: Sablier starts `migrate` whenever it is
  not running, i.e. on every request while the stack is awake, not once per wake.
- **The first request after a wake is answered `302 Found`** (a redirect to the same URL) by
  the plugin, not by the service; browsers follow it, `curl` without `-L` does not.
- **The plugin needs the network on every start of the proxy**: Traefik 3.7 refuses the
  plugin without `hash` ("plugin integrity check failed"; the catalog's validation answers
  404), and with the archive cached and `hash` pinned it still calls `plugins.traefik.io` on
  start and disables all plugins when it cannot reach it.
- **Files written by the proxy into `~/.siding/proxy/` become root-owned**, so siding running
  as the user cannot remove them without a helper container.

The gate applied; the maintainer decided to drop Sablier (D1). This design is the revision.

## Goals / Non-Goals

**Goals:**
- Memory from stacks nobody used for a while, taken only when a new stack needs it, never from
  active stacks, and without losing anything.
- A stopped stack is visible as such (`Stopped`) and is started again the way Compose starts it.
- Nothing new runs: no container, no daemon, no file of siding's; the record of requests is the
  proxy's own container log.

**Non-Goals:**
- Stopping a stack after a time without requests while nothing else needs memory (it needs
  something that runs all the time).
- Waking a stack on a request (see the spike); moves to "later" in the roadmap.
- Counting activity that does not pass the proxy (`docker compose exec`, a worker's jobs).
- A page for requests to a stopped stack.

## Decisions

### D1. No Sablier (maintainer's decision after the spike)
Making room is done by `up` alone, with `docker compose`; a stopped stack is started again by
`siding up`. Alternatives, asked: Sablier with restrictions (the user excludes one-off services
with a label and adds `restart: on-failure` for `depends_on`, siding downloads the plugin and
loads it as a local plugin to start offline: many footnotes, and still not Compose's order);
postponing the phase until Sablier keeps the order. Rejected before: siding's own wake service
(custom code, and a background service). The guardrails do not change.

### D2. The idle threshold (maintainer's decision: on by default)
Making room is on for every user, with a threshold of 5 minutes. `setup --idle <duration>`
(a Go duration, at least 10 seconds; the end-to-end test uses 10s) keeps it in seconds as the
label `dev.siding.idle` on `siding-proxy`; `--idle off` writes `dev.siding.idle=0`, which turns
it off; `--idle default` removes the label; without the flag the label is kept, as with
`--budget`. `Setup` gains `idle`: the duration (`5m0s`) or `off`. Alternative, asked: opt-in
with `setup --idle` (v0.3 behaviour until turned on). Stopping loses nothing, so on by default
costs a user at most a `siding up`.

### D3. The record of requests: the proxy's access log on stdout
Traefik writes a JSON access log to stdout next to its own log
(`--accesslog=true --accesslog.format=json --accesslog.fields.defaultmode=drop
--accesslog.fields.names.RouterName=keep --accesslog.fields.names.StartUTC=keep
--accesslog.fields.headers.defaultmode=drop`), so a line holds the router, the time and
Traefik's `level`, `msg` and `time`, and no URL, header or address (seen in the spike). The
proxy's compose file sets `logging: {driver: json-file, options: {max-size: 10m, max-file:
"3"}}`, so Docker bounds it at 30 MiB whatever the daemon's default driver is. Alternatives: a
file under `~/.siding/proxy/` (root-owned on Linux, see the spike, and rotation needs `USR1`);
Traefik's metrics (request counters without a time; siding would have to sample them in the
background); Sablier's sessions (dropped with D1).

### D4. The last request of a stack
`internal/idle` reads the proxy's container log through the Engine API (stdout only), keeps
the lines that parse as JSON with a `RouterName`, and maps `<router>@docker` to the project of
the running stacks by the longest project name that is a prefix followed by `-`, so
`shop-main-x-web` belongs to `shop-main-x`, not `shop-main`. The last request of a stack is the
latest of: its latest line, the newest `StartedAt` of its containers, the proxy's `StartedAt`
(a new proxy has an empty record, and a new stack has not had its chance yet). A proxy whose
command has no `--accesslog=true` (one of v0.3) has no record: nothing counts as idle, and `up`
says once on stderr that `siding setup` turns making room on. siding's own probes count as
requests, on purpose: an agent polling `status` is using the stack.

### D5. Making room (maintainer's decision: only idle stacks, longest idle first)
Where `up` checks the budget (`checkBudget`, and in the queue only the first in line), a stack
that does not fit triggers: with the threshold on and a record present, list the other running
or partial stacks whose last request is at least the threshold ago; sort them oldest first;
take the shortest prefix whose memory in use makes the new stack fit. If even all of them do
not, stop nothing. Otherwise stop each with `docker compose -p <project> stop` (by project name,
no compose file needed, as `down`), print `stopped idle stack <project> (no request for 20m,
1.0 GiB)` on stderr, read the memory again, and continue. `--force` skips this. In the queue
the log is read at most every 30 seconds, not on every two-second check. Alternatives, asked:
any other stack, least recently used first (could stop the stack another agent is testing);
none.

### D6. The verdict `Stopped` (maintainer's decision on the name)
`verdict.Evaluate` returns `Stopped` before anything else when no container is running,
restarting or starting, every exit code is 0, 137 or 143, and at least one service has a route
path or a healthcheck (a stack of one-off services that all exited 0 stays `Ready`, as in
v0.3). siding cannot tell who stopped a stack, so a stack stopped by hand is `Stopped` too;
both are started with `siding up`. `status` evaluates the containers before it probes and
never probes a `Stopped` stack; `status --wait` ends on it like on `Ready`; it exits 2 with the
stack's `url` and the hint `siding up --wait`. `ps` keeps its state `stopped`. Alternative,
asked: `Idle`, with a state `idle` in `ps` (claims to know why the stack is stopped).

### D7. The skill
The skill names `Stopped` under exit 2 with `siding up --wait --json` to start the stack again,
says that `siding status --json` holds the stack's `url` without starting anything, and that
only the addresses siding reports work, never `localhost:<port>` from the compose file. It
does not mention `--idle`: the threshold is the user's decision.

### D8. Labels, codes, documents, tests
New label `dev.siding.idle` (proxy), into the conventions of CLAUDE.md and
`openspec/config.yaml`. New verdict `Stopped`; `Setup` gains `idle`; no new error code.
Golden files: the proxy's compose file, `Status` with `Stopped`, `Setup` with `idle`. The
end-to-end test makes room with `--idle 10s` and a small budget.

## Risks / Trade-offs

- [A stack used without the proxy (tests through `docker compose exec`, a worker) counts as
  idle and can be stopped under pressure] → only under pressure and after the threshold; its
  next `status` says `Stopped` with the hint, and the skill tells the agent to run
  `siding up --wait`; `--idle 30m` or `--idle off` for such setups; noted in the README.
- [A request to a stopped stack gets a plain 404] → `ps`, `ui` and `status` show it as stopped;
  a page for it is a non-goal of this phase.
- [Reading up to 30 MiB of log on a full budget] → only when a stack does not fit, and at most
  every 30 seconds in the queue.
- [Traefik changes the fields of its JSON access log] → pinned image; a golden of the proxy's
  compose file and the end-to-end test read a real line.
- [`docker compose -p <project> stop` without a compose file] → Compose acts on the project's
  containers by label, as for `down`; checked in the end-to-end test.

## Migration Plan

The first `siding setup` of v0.4 recreates the proxy with the record (Compose sees the changed
file); running stacks keep running. Until then `up` makes no room and says so. No label, file or
document needs migrating. Rollback: `siding setup --idle off`.
