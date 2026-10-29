# Spec Delta

## Purpose

Defines the limit on image builds that run at the same time, so that many agents starting at
once do not stall the machine with parallel builds.

## ADDED Requirements

### Requirement: Build slots
`siding up` of a project with at least one service with `build:` SHALL hold one of N build
slots on this machine while `docker compose up` runs, waiting for a free one first, and print
on stderr that it waits. A project without `build:` MUST NOT need a slot. Waiting for a slot
counts against `--timeout`.

#### Scenario: Two builds, one slot
- **WHEN** N is 1 and two `up`s of projects with `build:` start at the same moment
- **THEN** one runs `docker compose up` while the other waits, then the other runs

#### Scenario: No build
- **WHEN** all build slots are held and `up` runs for a project of images only
- **THEN** it starts without waiting

#### Scenario: A killed build
- **WHEN** the process holding a slot is killed
- **THEN** the slot is free again at once

### Requirement: Number of slots
N SHALL be the label `dev.siding.builds` on `siding-proxy`, set with `siding setup --builds N`
and kept by later `setup`s, and half the CPUs of this machine (at least 1) without the label.

#### Scenario: Default
- **WHEN** the machine has 8 CPUs and the proxy has no builds label
- **THEN** N is 4
