# stack-lifecycle Specification

## Purpose
Defines how a worktree's stack is started, inspected through its logs and taken down: the
user's compose file stays untouched, siding adds one override, and nothing removes unmerged or
uncommitted work without being forced.

## Requirements

### Requirement: Compose file
siding SHALL use the compose file Compose itself would pick in the worktree's top-level
directory (`compose.yaml`, `compose.yml`, `docker-compose.yaml`, `docker-compose.yml`), or the
one named by `--file`. Without one, stack commands MUST fail with `ComposeFileNotFound`.

#### Scenario: The default file
- **WHEN** the worktree holds `compose.yaml`
- **THEN** `siding up` uses it

#### Scenario: No compose file
- **WHEN** the worktree holds no compose file and `--file` is not given
- **THEN** `siding up` fails with `ComposeFileNotFound`

### Requirement: The compose file is never edited
siding MUST NOT write to the user's compose file, its `.env` or any other file of the user. It
SHALL write only `.siding/override.yml` and `.siding/.gitignore` (containing `*`) in the
worktree, so that `.siding/` never makes the worktree dirty.

#### Scenario: After up
- **WHEN** `siding up` has run in a clean worktree
- **THEN** `git status --porcelain` prints nothing and the compose file is byte for byte unchanged

### Requirement: Override
The override SHALL reset every service's `ports` (`ports: !reset []`), add every routed
service to the network `siding` next to its own networks, and add the `dev.siding.*` and
Traefik labels. The user's labels and settings MUST otherwise stay in effect.

#### Scenario: A published port
- **WHEN** the compose file publishes `"5432:5432"` for `db`
- **THEN** after `siding up` no container of the stack publishes a port

#### Scenario: Own networks
- **WHEN** `api` is on the user's networks `backend` and `default`
- **THEN** after `siding up` it is on `backend`, `default` and `siding`

### Requirement: Up
`siding up` SHALL run `docker compose -p <project> -f <compose file> -f .siding/override.yml
up -d --remove-orphans`, followed by any arguments after `--`. It SHALL print the stack's
address, with `--json` as a `Stack` document, and fail with `ComposeFailed` and Compose's
message when Compose fails.

#### Scenario: Passing arguments through
- **WHEN** `siding up -- --build web` runs
- **THEN** Compose builds and starts `web` and its dependencies

#### Scenario: Compose fails
- **WHEN** an image of the compose file does not exist
- **THEN** `siding up` exits 1 with `ComposeFailed` and Compose's error in the message

#### Scenario: Run twice
- **WHEN** `siding up` runs again in the same worktree
- **THEN** the same project is updated, and no second stack appears

### Requirement: Up and wait
`siding up --wait [--timeout <d>]` SHALL start the stack as `up` does and then end exactly like
`siding status --wait`, with its document and exit code.

#### Scenario: An agent starts and waits
- **WHEN** an agent runs `siding up --wait --json` and every service becomes ready
- **THEN** it prints one `Status` document with the verdict Ready and exits 0

### Requirement: Down
`siding down` SHALL remove the stack's containers and networks by project name, also when the
compose file is gone, and keep its volumes. A stack whose containers are already gone but whose
volumes remain SHALL still be found by the labels of its volumes, so its data can be removed. It
SHALL print what it removed, with `--json` as a `Down` document, and with `--dry-run` remove
nothing.

#### Scenario: Data survives
- **WHEN** `siding down` and then `siding up` run
- **THEN** the stack's named volumes still hold their data

#### Scenario: Dry run
- **WHEN** `siding down --dry-run --json` runs
- **THEN** it prints a `Down` document listing the containers and networks with `dryRun: true`, and everything still runs

#### Scenario: Nothing running
- **WHEN** `siding down` runs in a worktree whose stack has neither containers nor volumes
- **THEN** it fails with `StackNotFound`

#### Scenario: Only the data is left
- **WHEN** `siding down` ran before, and now `siding down --volumes` runs in the same worktree
- **THEN** it finds the stack by its volumes and removes them, with the guard for unmerged and dirty work

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

### Requirement: Logs
`siding logs [service…]` SHALL show the logs of the stack's services, following them with
`-f` and limited with `--tail <n>`. With `--json` it SHALL print a `Logs` document of the last
lines per service; `--json` together with `-f` MUST fail with `InvalidArguments`.

#### Scenario: One service
- **WHEN** `siding logs api --tail 5` runs
- **THEN** it prints the last 5 lines of `api`

### Requirement: No .env values in JSON
Log lines in any document SHALL have every value defined in the worktree's `.env` and in the
services' `env_file`s, of 4 or more characters, replaced by `***`.

#### Scenario: A password in a log line
- **WHEN** `.env` sets `DB_PASSWORD=not-a-real-token` and a service logs that value
- **THEN** the log line in the `Logs` and `Status` documents shows `***` instead
