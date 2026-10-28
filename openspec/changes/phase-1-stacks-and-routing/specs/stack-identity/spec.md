# Spec Delta

## Purpose

Defines how siding derives a stack's identity from a git worktree: repository name, branch
slug, Compose project, host names, machine id and the labels that let Docker answer every
later question about the stack.

## ADDED Requirements

### Requirement: One stack per worktree
siding SHALL treat the git worktree containing the current directory as the stack's worktree,
identified by its absolute top-level path. Every command that acts on a stack MUST act on that
worktree's stack.

#### Scenario: Run from a subdirectory
- **WHEN** `siding status` runs in `/src/shop-login/web/src`, inside the worktree `/src/shop-login`
- **THEN** it reports the stack of `/src/shop-login`

#### Scenario: Outside a repository
- **WHEN** a stack command runs outside any git worktree
- **THEN** it fails with `NotAGitRepository`

### Requirement: Repository name
The repository name SHALL be the name of the directory that holds the repository's common git
directory (the main worktree), the same for every worktree of the repository. `--repo <name>`
MUST override it.

#### Scenario: Two worktrees of one repository
- **WHEN** `/src/shop` is the main worktree and `/src/shop-login` a linked worktree of it
- **THEN** both have the repository name `shop`

#### Scenario: Override
- **WHEN** `siding up --repo store` runs in `/src/shop`
- **THEN** the repository name is `store`

### Requirement: Slugs
The repository name and the branch SHALL each be turned into a slug: lowercase, every run of
characters other than `a-z` and `0-9` replaced by one `-`, no leading or trailing `-`. A slug
longer than 40 characters MUST be cut to 33 characters, followed by `-` and the first 6 hex
digits of the SHA-256 of the original name.

#### Scenario: A branch with a slash
- **WHEN** the branch is `feature/Login_Form`
- **THEN** its slug is `feature-login-form`

#### Scenario: A long branch
- **WHEN** the branch name is 60 characters long
- **THEN** its slug is 40 characters long and ends in `-` and 6 hex digits that differ for another branch with the same first 33 characters

### Requirement: Branch required
The worktree SHALL be on a branch. In a detached HEAD a stack command MUST fail with `NoBranch`
and a hint to check out a branch.

#### Scenario: Detached HEAD
- **WHEN** `siding up` runs in a worktree whose HEAD is detached
- **THEN** it fails with `NoBranch` and starts nothing

### Requirement: Project name and host
The Compose project SHALL be named `<repo-slug>-<branch-slug>` and the stack's host SHALL be
`<repo-slug>.<branch-slug>.localhost`.

#### Scenario: Names
- **WHEN** the repository is `shop` and the branch `feature/login`
- **THEN** the project is `shop-feature-login` and the host `shop.feature-login.localhost`

### Requirement: Slug collision
siding MUST NOT let two worktrees or branches share one project. If the project already exists
with another `dev.siding.worktree` or `dev.siding.branch` label, `up` MUST fail with
`SlugCollision`, naming the other worktree and branch, and change nothing.

#### Scenario: feature/a and feature-a
- **WHEN** a stack for branch `feature/a` runs and `siding up` runs in a worktree on branch `feature-a`
- **THEN** it fails with `SlugCollision` and the running stack is untouched

### Requirement: Labels
siding SHALL label every container, network and volume of a stack with `dev.siding.repo`,
`dev.siding.branch` (the branch name, not the slug), `dev.siding.worktree` (absolute path) and
`dev.siding.machine`, and its containers with `dev.siding.agent` when `--agent` or
`SIDING_AGENT` names one. External networks and volumes MUST NOT be relabelled.

#### Scenario: Labels on a running stack
- **WHEN** `siding up` has run in `/src/shop-login` on branch `feature/login`
- **THEN** every container, the project's networks and its named volumes carry `dev.siding.repo=shop`, `dev.siding.branch=feature/login` and `dev.siding.worktree=/src/shop-login`

#### Scenario: An agent name
- **WHEN** `SIDING_AGENT=claude siding up` runs
- **THEN** the containers also carry `dev.siding.agent=claude`

### Requirement: Machine id
`dev.siding.machine` SHALL be a stable id of the machine siding runs on: derived from the
operating system's machine id by a one-way hash, never the raw id. Without an operating system
id, siding MUST generate one and keep it in `~/.siding/machine-id`.

#### Scenario: Two runs
- **WHEN** `siding up` runs twice on the same machine
- **THEN** both stacks carry the same `dev.siding.machine`

#### Scenario: No operating system id
- **WHEN** neither `/etc/machine-id` nor the macOS platform UUID is available
- **THEN** siding writes a random id to `~/.siding/machine-id` and uses it from then on
