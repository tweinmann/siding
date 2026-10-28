# Spec Delta

## MODIFIED Requirements

### Requirement: Removing data keeps work
`siding down --volumes` SHALL also remove the stack's volumes, but MUST refuse with
`UnmergedBranch` when the branch is neither merged into the default branch nor has a deleted
upstream branch, and with `DirtyWorktree` when `git status` lists changes, unless `--force` is
given.

#### Scenario: Unmerged branch
- **WHEN** `siding down --volumes` runs on a branch with commits that the default branch lacks
- **THEN** it fails with `UnmergedBranch`, removes nothing, and the hint names `--force`

#### Scenario: Dirty worktree
- **WHEN** the worktree has uncommitted changes and `siding down --volumes` runs
- **THEN** it fails with `DirtyWorktree` and removes nothing

#### Scenario: Forced
- **WHEN** `siding down --volumes --force` runs on an unmerged branch
- **THEN** the containers, networks and volumes of the stack are removed

#### Scenario: No default branch
- **WHEN** siding finds neither `origin/HEAD` nor a local `main` or `master`
- **THEN** the branch counts as unmerged

#### Scenario: Squash-merged
- **WHEN** the branch's PR was squash-merged, its remote branch deleted and `git fetch --prune` ran
- **THEN** `siding down --volumes` removes the volumes without `--force`
