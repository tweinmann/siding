# Spec Delta

## ADDED Requirements

### Requirement: Env files of the main worktree
In a linked worktree, `up` SHALL fail with `EnvFileMissing` before Compose runs when the
worktree lacks `.env`, or a file the compose file names in `env_file`, that the main worktree
has and git does not track. The hint MUST give the `cp` command from the main worktree. siding
MUST NOT copy the file itself.

#### Scenario: A new worktree without .env
- **WHEN** the main worktree `/src/shop` has an untracked `.env`, and `siding up` runs in the new worktree `/src/shop-feature-x`, which has none
- **THEN** it fails with `EnvFileMissing`, the hint says `cp /src/shop/.env /src/shop-feature-x/.env`, and no container starts

#### Scenario: An env_file
- **WHEN** `api` has `env_file: [api.env]`, the main worktree has an untracked `api.env`, and the worktree lacks it
- **THEN** `siding up` fails with `EnvFileMissing` naming `api.env`

#### Scenario: Not needed
- **WHEN** the main worktree has no `.env`, or `.env` is tracked by git
- **THEN** `up` behaves as before

#### Scenario: The main worktree
- **WHEN** `siding up` runs in the main worktree without `.env`
- **THEN** `up` behaves as before
