# Spec Delta

## Purpose

Defines how stacks that do not fit into the memory budget wait their turn instead of being
refused, without a background service.

## ADDED Requirements

### Requirement: Waiting in line
When a stack does not fit into the budget, `siding up --wait` SHALL wait in line instead of
refusing, until it fits or `--timeout` is up, and then start it as usual. Without `--wait`,
`up` SHALL refuse with `NoCapacity` and exit 3 as before.

#### Scenario: Memory frees
- **WHEN** `siding up --wait` waits because the budget is full, and another stack is taken down
- **THEN** the waiting stack starts within a few seconds and the command ends with its verdict

#### Scenario: Time is up
- **WHEN** nothing frees memory before `--timeout`
- **THEN** `up` exits 3 with `NoCapacity`, its place in line and the memory it needs, and starts nothing

### Requirement: First come, first served
Stacks SHALL start in the order they began to wait: only the first one in line may start, even
if a later one would fit. A waiting `up` that ended (stopped, killed, timed out) MUST leave the
line.

#### Scenario: Order
- **WHEN** `a` began to wait before `b`, and memory frees enough for `b` but not for `a`
- **THEN** `b` keeps waiting until `a` has started

#### Scenario: A killed waiter
- **WHEN** the process of the first waiting `up` was killed
- **THEN** the next one moves up within a few seconds

### Requirement: The line is visible
`siding ps` and `siding ui` SHALL list the stacks waiting in line with project, place, memory
needed and since when they wait, and the waiting `up` SHALL print its place on stderr when it
changes.

#### Scenario: ps while waiting
- **WHEN** `shop-feature-x` waits in line
- **THEN** `siding ps` shows it under "waiting" with its place and the memory it needs
