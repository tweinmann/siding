# Proposal

## Why

Most stacks sit idle most of the time: a branch waits for review, an agent works on code, not on
the running app. Every idle stack still holds its memory, so the budget fills with stacks
nobody uses, and the next `up` waits or is refused. Stopping idle stacks and waking them on the
next request frees that memory without the developer or the agent noticing more than a slower
first request.

## What Changes

- `siding setup --idle <duration>` turns on idle stop for every stack on the machine; `--idle
  off` turns it off. Without the flag nothing changes from v0.3. The setting is a label on
  `siding-proxy`, like the budget and the build slots.
- With idle stop on, the proxy project runs a second container next to `siding-proxy`: Sablier,
  with its Traefik plugin. A stack that had no request through the proxy for the idle duration
  is stopped (containers only, nothing is removed); the next request to any of its addresses
  starts it again and is answered once the stack is up.
- A sleeping stack has its own state: `siding status` reports the verdict `Idle` (exit 2) with
  its address, without waking it; `siding ps` and `siding ui` show the state `idle`.
  `siding up` wakes it as it starts any stopped stack.
- Under memory pressure, `siding up` first stops other stacks that are idle-enabled and had no
  request for at least 5 minutes, longest idle first, when that makes the new stack fit; only
  then does it wait in line or refuse as before.
- Agent skill: the verdict `Idle` and how to wake a stack; `siding status --json` gives the
  stack's `url` without restarting it; the agent never gives `localhost:<port>` from the compose
  file, because siding publishes no ports.
- The guardrail "no background service" is made precise: siding itself still runs no daemon;
  the proxy project `siding` runs standard building blocks (Traefik, now Sablier), as Docker
  containers.

## Capabilities

### New Capabilities
- `idle-stop`: the idle setting, stopping a stack after the idle duration, waking it on a
  request, and making room under memory pressure by stopping idle stacks.

### Modified Capabilities
- `proxy`: `setup` reports the idle setting and runs Sablier while idle stop is in use;
  `teardown` removes Sablier and its files too.
- `stack-status`: the verdict `Idle`; `status --wait` ends at once on it.
- `stack-list`: the state `idle`.
- `memory-budget`: `up` refuses only after idle stacks could not make room.
- `cli-contract`: `Idle` exits 2.
- `agent-skill`: what the skill teaches about `Idle`, the address from `status`, and no
  `localhost:<port>`.
- `dev-environment`: `just smoke-stacks` also covers sleeping and waking.

## Impact

- Code: `internal/proxy` (Sablier service, plugin, access log in the proxy's compose file),
  `internal/stack` (Sablier labels and middleware in the override), `internal/verdict` (`Idle`),
  `internal/cli` (`setup --idle`, `status`, `ps`, `up` making room), a new `internal/idle`
  (last request per stack from the access log), `internal/e2e`.
- Documents: `Setup` gains `idleSeconds`; `Status` can carry `verdict: Idle`; `StackList`
  entries can carry `state: idle`. Only additions.
- Dependencies: the image `sablierapp/sablier` and the Traefik plugin
  `github.com/sablierapp/sablier-traefik-plugin`, both pinned; the plugin is downloaded by
  Traefik when the proxy starts with idle stop on. Traefik v3.6 or later (pinned: v3.7.16).
- Files under `~/.siding/proxy/<project>/`: the plugin cache, Sablier's sessions and the
  proxy's access log, all losable.
- CLAUDE.md, README and `openspec/config.yaml`: the precise guardrail, the label
  `dev.siding.idle`, the Sablier labels in the override, the container `siding-sablier`.
