# Spec Delta

## Purpose

Defines `siding ui`, the one page where a human sees every stack on the machine, what it costs
against the budget, and what could go after a merge, served only while the command runs.

## ADDED Requirements

### Requirement: Overview in the foreground
`siding ui` SHALL serve a read-only overview until it is stopped, at `http://siding.localhost`
through the proxy and at the local address it prints. When it stops, the route MUST go away and
nothing of it MUST keep running. It SHALL NOT change any stack.

#### Scenario: Start and stop
- **WHEN** `siding ui` runs and the user presses Ctrl-C
- **THEN** while it ran `http://siding.localhost` showed the overview, and afterwards the proxy answers 404 for `siding.localhost` and no process of siding is left

#### Scenario: No proxy
- **WHEN** `siding ui` runs while `siding-proxy` is not running or lacks the overview route
- **THEN** it fails with `ProxyNotRunning` and the hint `siding setup`

### Requirement: Only this machine
The overview SHALL listen only on addresses of this machine that the proxy can reach (the
loopback interface, or the proxy network's gateway when the Docker daemon runs on this
machine), on `--port` or a free port, and MUST NOT listen on all interfaces.

#### Scenario: Not on the network
- **WHEN** `siding ui` runs on a Linux machine with a LAN address
- **THEN** a request to that LAN address and the overview's port is refused

### Requirement: What the page shows
The page SHALL show every stack as `siding ps` lists it (repository, branch, state, address as
a link, memory, worktree), the memory in use against the budget, and the stacks `siding gc`
would remove, with the reason. It SHALL refresh itself every 5 seconds.

#### Scenario: Two stacks and a merged branch
- **WHEN** stacks run for `main` and for a merged, clean `feature/login`
- **THEN** the page lists both with links to their addresses, the total against the budget, and `feature/login` under "can go" with the reason `Merged`

### Requirement: JSON endpoints
The overview SHALL answer `GET /api/stacks` with the `StackList` document of `siding ps --json`
and `GET /api/gc` with the `GC` document of `siding gc --dry-run --json`, so the page and
scripts read the same data. Every other method MUST be refused with 405.

#### Scenario: Same document as ps
- **WHEN** `GET http://siding.localhost/api/stacks` is requested
- **THEN** the body is a `StackList` document equal to what `siding ps --json` prints at that moment

#### Scenario: No actions
- **WHEN** `POST http://siding.localhost/api/gc` is requested
- **THEN** the overview answers 405 and removes nothing
