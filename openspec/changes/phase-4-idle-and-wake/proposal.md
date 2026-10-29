# Proposal

## Why

Most stacks sit idle most of the time: a branch waits for review, an agent works on code, not on
the running app. Every idle stack still holds its memory, so the budget fills with stacks
nobody uses, and the next `up` waits or is refused. When a new stack needs the memory, stopping
stacks nobody requested for a while gives it back without losing anything.

## What Changes

- Under memory pressure, `siding up` first stops other stacks that had no request through the
  proxy for the idle threshold (5 minutes), longest idle first, when that makes the new stack
  fit; only then does it wait in line or refuse as before. Nothing is removed: containers are
  stopped, volumes and networks stay.
- `siding setup --idle <duration>` changes the threshold, `--idle off` turns making room off;
  the setting is a label on `siding-proxy`, like the budget and the build slots.
- The proxy records, per request, only the router and the time, in its container log, which
  Docker keeps at a bounded size. That record is where the last request of a stack comes from.
- A stopped stack has a verdict of its own: `siding status` reports `Stopped` (exit 2) with its
  address and the hint `siding up --wait`, instead of a `TimedOut` or `Starting` that would make
  an agent look for a bug.
- A stopped stack is started again by `siding up` in its worktree, in Compose's order; a request
  to it does not start it.
- Agent skill: the verdict `Stopped` and how to start the stack again; `siding status --json`
  gives the stack's `url` without starting anything; the agent never gives `localhost:<port>`
  from the compose file, because siding publishes no ports.

Not in this phase, after the spike of task 1.1 (see design.md): stopping a stack after a time
without requests, and waking it on a request. Sablier, the standard building block for both,
ignores Compose's `depends_on` order on wake, starts one-off services again on every request, and
needs the network on every start of the proxy.

## Capabilities

### New Capabilities
- `idle-stop`: the idle threshold, the last request of a stack, stopping idle stacks to make
  room, and starting a stopped stack again.

### Modified Capabilities
- `proxy`: `setup` reports the idle threshold; the proxy records router and time per request.
- `stack-status`: the verdict `Stopped`; `status --wait` ends at once on it.
- `memory-budget`: `up` refuses only after idle stacks could not make room; stopped stacks use
  nothing.
- `cli-contract`: `Stopped` exits 2.
- `agent-skill`: what the skill teaches about `Stopped`, the address from `status`, and no
  `localhost:<port>`.
- `dev-environment`: `just smoke-stacks` also covers making room.

## Impact

- Code: `internal/proxy` (access log to stdout with two fields, bounded `json-file` logging),
  `internal/verdict` (`Stopped`), `internal/cli` (`setup --idle`, `status`, `up` making room), a
  new `internal/idle` (last request per stack from the proxy's log), `internal/docker` (container
  logs with timestamps, stopping a project), `internal/e2e`.
- Documents: `Setup` gains `idle`; `Status` can carry `verdict: Stopped`. Only additions.
- The proxy is recreated by the first `siding setup` of v0.4; running stacks keep running.
- No new dependency, image, container or file under `~/.siding`.
- CLAUDE.md, README and `openspec/config.yaml`: the label `dev.siding.idle`, the roadmap (wake on
  request moves to "later").
