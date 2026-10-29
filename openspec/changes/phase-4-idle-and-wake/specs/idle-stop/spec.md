# Spec Delta

## Purpose

Defines how stacks nobody uses give their memory back: idle stop after a duration without
requests through the proxy, waking on the next request, and stopping idle stacks to make room
for a new one.

## ADDED Requirements

### Requirement: Idle setting
`siding setup --idle <duration>` (`30m`, `2h`, at least `10s`) SHALL turn on idle stop for every
stack on the machine, kept as the label `dev.siding.idle` (seconds) on `siding-proxy`;
`--idle off` SHALL remove it. A `setup` without `--idle` MUST keep the setting it finds. Without
the label, siding SHALL behave as without idle stop.

#### Scenario: Turn it on
- **WHEN** `siding setup --idle 30m` runs
- **THEN** `siding-proxy` carries `dev.siding.idle=1800`, and running stacks keep running

#### Scenario: setup keeps it
- **WHEN** `siding setup --idle 30m` and later `siding setup --port 8080` run
- **THEN** `siding-proxy` still carries `dev.siding.idle=1800`

#### Scenario: Too short
- **WHEN** `siding setup --idle 5s` runs
- **THEN** it fails with `InvalidArguments` and changes nothing

### Requirement: Stacks take part from their next up
While idle stop is on, `siding up` SHALL make the stack idle-enabled: its containers can be
stopped for idleness and woken by a request. A stack started before idle stop was turned on,
or started while it is off, SHALL NOT be stopped for idleness until its next `up`.

#### Scenario: An older stack
- **WHEN** `shop-main` runs since before `siding setup --idle 30m`, and gets no request for an hour
- **THEN** it keeps running until `siding up` runs again in its worktree

#### Scenario: Turned off
- **WHEN** `siding setup --idle off` runs while idle-enabled stacks exist
- **THEN** those stacks keep sleeping and waking as before, setup names them with the hint to run `siding up` in their worktrees, and stacks started afterwards are not idle-enabled

### Requirement: Sleeping after the idle duration
An idle-enabled stack that had no request through the proxy for the idle duration SHALL be
stopped: every container of the stack, routed or not, is stopped, and nothing is removed. A
stack without a routed service SHALL never be stopped for idleness.

#### Scenario: Idle
- **WHEN** the idle duration is 30m and nothing requested any address of `shop-feature-x` for 30 minutes
- **THEN** every container of `shop-feature-x` is stopped, and its volumes and networks remain

#### Scenario: Any address counts
- **WHEN** only `http://api.shop.feature-x.localhost` was requested in the last 30 minutes
- **THEN** the whole stack keeps running

#### Scenario: No route
- **WHEN** a stack has only services with `dev.siding.route: "false"` or without an HTTP port
- **THEN** it is never stopped for idleness

#### Scenario: Never requested
- **WHEN** `siding up` started an idle-enabled stack and nothing requested it afterwards
- **THEN** it is stopped once the idle duration has passed after that `up`

### Requirement: Waking on a request
A request through the proxy to any address of a sleeping stack SHALL start every container of
the stack and be answered by the service once the stack is up, healthy where it has
healthchecks; when it is not up within 2 minutes, the proxy SHALL answer 503. The request MUST
NOT be answered by a waiting page.

#### Scenario: Browser after lunch
- **WHEN** `shop-feature-x` sleeps and the user opens `http://shop.feature-x.localhost:8080/checkout`
- **THEN** the stack starts, and the page is answered by `web` once the stack is up

#### Scenario: Agent with curl
- **WHEN** an agent requests `http://api.shop.feature-x.localhost:8080/health` of a sleeping stack
- **THEN** the response comes from `api`, with the header `X-Siding-Service: shop-feature-x/api`

#### Scenario: up wakes it too
- **WHEN** `siding up --wait` runs in the worktree of a sleeping stack
- **THEN** the stack starts and the command ends with its verdict, as for any stopped stack

### Requirement: Idle stacks make room
When a new stack does not fit into the budget, `siding up` SHALL first stop other idle-enabled
stacks that had no request through the proxy for at least 5 minutes, longest idle first, as
many as the new stack needs, but only when stopping them makes it fit. It SHALL name every
stack it stopped on stderr. The worktree's own stack MUST NOT be stopped.

#### Scenario: Room from an idle stack
- **WHEN** the budget is 4 GiB, `shop-main` (1 GiB, idle-enabled, last request 20 minutes ago) and `shop-fix` (2.5 GiB, last request 1 minute ago) run, and the new stack needs 1 GiB
- **THEN** `siding up` stops `shop-main`, says so on stderr, and starts the new stack

#### Scenario: Not enough even then
- **WHEN** stopping every idle stack would still not make the new stack fit
- **THEN** `siding up` stops nothing and waits in line with `--wait` or refuses with `NoCapacity` as before

#### Scenario: In line
- **WHEN** `siding up --wait` waits in line and an idle-enabled stack passes 5 minutes without a request
- **THEN** the first in line stops it when that makes it fit, and starts

#### Scenario: Not idle-enabled
- **WHEN** a stack started before idle stop was turned on had no request for an hour
- **THEN** `siding up` does not stop it to make room

#### Scenario: Unknown last request
- **WHEN** siding cannot tell when a stack was last requested, because the record of requests is lost
- **THEN** it does not stop that stack to make room
