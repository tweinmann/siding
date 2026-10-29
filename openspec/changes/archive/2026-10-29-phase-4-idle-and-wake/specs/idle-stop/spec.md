# Spec Delta

## Purpose

Defines how stacks nobody uses give their memory back when a new stack needs it: which stacks
count as idle, how `up` stops them to make room, and how a stopped stack is started again.

## ADDED Requirements

### Requirement: Idle threshold
A stack SHALL count as idle when it had no request through the proxy for the idle threshold,
5 minutes unless `siding setup --idle <duration>` (at least `10s`) sets another, kept as the
label `dev.siding.idle` (seconds) on `siding-proxy`. `--idle off` SHALL turn making room off,
`--idle default` SHALL remove the label, and a `setup` without `--idle` MUST keep it.

#### Scenario: Default
- **WHEN** the proxy carries no `dev.siding.idle`
- **THEN** a stack without a request for 5 minutes counts as idle

#### Scenario: Another threshold
- **WHEN** `siding setup --idle 15m` runs, and later `siding setup --port 8080`
- **THEN** `siding-proxy` carries `dev.siding.idle=900` after both

#### Scenario: Off
- **WHEN** `siding setup --idle off` runs
- **THEN** `siding up` never stops another stack, and behaves as v0.3

#### Scenario: Too short
- **WHEN** `siding setup --idle 5s` runs
- **THEN** it fails with `InvalidArguments` and changes nothing

### Requirement: Last request of a stack
The last request of a stack SHALL be the latest request through the proxy to any of its
addresses. When the proxy has no record of a request since the stack or the proxy last
started, the later of those two starts SHALL count as its last request.

#### Scenario: Any address counts
- **WHEN** only `http://api.shop.feature-x.localhost` was requested 2 minutes ago
- **THEN** the last request of `shop-feature-x` is 2 minutes ago

#### Scenario: Never requested
- **WHEN** `shop-feature-x` was started 20 minutes ago and never requested
- **THEN** its last request counts as 20 minutes ago

#### Scenario: The proxy was recreated
- **WHEN** `siding setup --port 8080` recreated the proxy 1 minute ago
- **THEN** no stack counts as idle before the threshold has passed after that

### Requirement: Idle stacks make room
When a new stack does not fit into the budget, `siding up` SHALL first stop other running
stacks that are idle, longest idle first, as many as the new stack needs, but only when
stopping them makes it fit. It SHALL stop them without removing anything and name every stack
it stopped on stderr. The worktree's own stack MUST NOT be stopped; `--force` stops none.

#### Scenario: Room from an idle stack
- **WHEN** the budget is 4 GiB, `shop-main` (1 GiB, last request 20 minutes ago) and `shop-fix` (2.5 GiB, last request 1 minute ago) run, and the new stack needs 1 GiB
- **THEN** `siding up` stops the containers of `shop-main`, says so on stderr, and starts the new stack

#### Scenario: Not enough even then
- **WHEN** stopping every idle stack would still not make the new stack fit
- **THEN** `siding up` stops nothing and waits in line with `--wait` or refuses with `NoCapacity` as before

#### Scenario: In line
- **WHEN** `siding up --wait` waits in line and another stack becomes idle
- **THEN** the first in line stops it when that makes it fit, and starts

#### Scenario: A proxy without the record
- **WHEN** `siding-proxy` was created by siding v0.3, so it records no requests
- **THEN** `siding up` stops nothing, says on stderr that `siding setup` turns making room on, and waits or refuses as before

#### Scenario: Data is kept
- **WHEN** `siding up` stopped `shop-main` to make room
- **THEN** the volumes and networks of `shop-main` remain, and `siding up --wait` in its worktree starts it again with its data

### Requirement: Starting a stopped stack again
A stack stopped to make room SHALL be started again by `siding up` in its worktree, in the
order Compose starts it. A request through the proxy to a stopped stack MUST NOT start it.

#### Scenario: up starts it
- **WHEN** `shop-main` was stopped to make room and `siding up --wait` runs in its worktree
- **THEN** Compose starts its services in their `depends_on` order and the command ends with the verdict

#### Scenario: A request does not
- **WHEN** a browser requests `http://shop.main.localhost` while `shop-main` is stopped
- **THEN** the proxy answers 404 and the stack stays stopped
