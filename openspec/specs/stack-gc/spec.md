# stack-gc Specification

## Purpose
Defines `siding gc`, which removes the stacks that can go after a merge or a deleted worktree,
and never anything that could hold unmerged or uncommitted work.

## Requirements

### Requirement: Merged
A branch SHALL count as merged for `gc` when it is not the default branch, its worktree has no
changes, and either its tip is contained in the default branch while the default branch has
moved past it, or its upstream branch is gone (deleted after a merge, a squash merge included).

#### Scenario: Merged and the default branch moved on
- **WHEN** `feature/login` was merged into `main`, `main` has a commit after it, and the worktree is clean
- **THEN** `feature/login` counts as merged

#### Scenario: A fresh branch
- **WHEN** an agent just created `feature/new` from `main`, without a commit of its own
- **THEN** it does not count as merged, and gc leaves its stack alone

#### Scenario: Squash merge
- **WHEN** the PR of `feature/x` was squash-merged and the remote branch deleted, and `git fetch --prune` ran
- **THEN** `feature/x` counts as merged, although its commits are not in `main`

#### Scenario: The default branch
- **WHEN** a stack runs for `main`
- **THEN** gc never removes it

### Requirement: What gc removes
`siding gc` SHALL consider the stacks of this machine and remove: for a merged branch, the
stack's containers, networks and volumes (reason `Merged`); for a stack whose worktree no
longer exists, its containers and networks, and its volumes only if the branch is merged
(reason `WorktreeGone`). It MUST NOT touch git worktrees, branches or snapshots.

#### Scenario: A deleted worktree with an unmerged branch
- **WHEN** the worktree of `feature/wip` was deleted, but the branch has commits `main` lacks
- **THEN** gc removes its containers and networks, keeps its volumes, and reports them as kept

#### Scenario: A stack from an older siding
- **WHEN** a stack without the label `dev.siding.gitdir` has lost its worktree
- **THEN** gc removes its containers and networks and keeps its volumes, since it cannot tell whether the branch is merged

#### Scenario: Work in progress
- **WHEN** a worktree has uncommitted changes or its branch is not merged
- **THEN** gc removes nothing of its stack and lists it as kept with the reason `Dirty` or `Unmerged`

#### Scenario: Worktrees stay
- **WHEN** gc removes the stack of a merged branch
- **THEN** the worktree directory and the branch still exist

#### Scenario: Snapshots stay
- **WHEN** gc removes the stack of a merged branch from which the snapshot `base` was saved
- **THEN** `base` still exists

### Requirement: Ask before removing
`siding gc` SHALL list what it would remove and ask before removing it. `--yes` removes without
asking, and `--dry-run` only lists. Without a terminal, or with `--json`, it MUST fail with
`NotInteractive` unless `--yes` or `--dry-run` is given.

#### Scenario: Interactive
- **WHEN** `siding gc` runs in a terminal and the user answers anything but `y`
- **THEN** nothing is removed

#### Scenario: An agent runs gc
- **WHEN** `siding gc --json` runs without `--yes` or `--dry-run`
- **THEN** it fails with `NotInteractive` and removes nothing

### Requirement: GC document
With `--json`, gc SHALL print one `GC` document: `dryRun`, the stacks it removes (or would)
with project, repository, branch, worktree, reason, and the containers, networks and volumes
removed and kept, and the stacks it keeps with their reason.

#### Scenario: Dry run as JSON
- **WHEN** `siding gc --dry-run --json` runs with one merged stack and one dirty one
- **THEN** it prints a `GC` document with `dryRun: true`, the merged stack under `stacks` with reason `Merged`, and the dirty one under `kept` with reason `Dirty`
