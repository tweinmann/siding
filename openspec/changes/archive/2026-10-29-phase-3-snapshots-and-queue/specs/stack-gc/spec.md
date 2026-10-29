# Spec Delta

## MODIFIED Requirements

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
