# compose-lint Specification

## Purpose
Defines `siding lint`, which tells a user before the first `up` what in their compose file
breaks parallel stacks or weakens the verdict, without changing the file.

## Requirements

### Requirement: Lint reads, never writes
`siding lint [--file]` SHALL check the compose file siding would use, through the configuration
Compose prints, and MUST NOT change any file. It works in any directory with a compose file,
inside a git worktree or not, and needs no running proxy.

#### Scenario: Outside a repository
- **WHEN** `siding lint` runs in a directory with `compose.yaml` that is not a git worktree
- **THEN** it checks that file

### Requirement: Findings
Every finding SHALL have a rule, a severity (`error`, `warning` or `info`), the service or
resource it is about, a message and a hint. `lint` SHALL exit 1 when there is an error and 0
otherwise, and with `--json` print one `LintReport` document with the file and its findings.

#### Scenario: A clean file
- **WHEN** the compose file has no finding
- **THEN** `siding lint` prints that it found nothing and exits 0

#### Scenario: Errors as JSON
- **WHEN** `siding lint --json` finds an error
- **THEN** it prints a `LintReport` with the finding and exits 1

### Requirement: Errors
`lint` SHALL report as errors what makes two stacks of the same file collide or `up` refuse:
`ContainerName` (a fixed `container_name`), `FixedNetworkName` (an own network with a fixed
`name`), `FixedVolumeName` (an own volume with a fixed `name`, unless it carries
`dev.siding.shared`), and `InvalidRoute` and `AmbiguousPort` as `up` reports them.

#### Scenario: A fixed container name
- **WHEN** `db` sets `container_name: shop-db`
- **THEN** lint reports `ContainerName` for `db` with the hint to remove it, since a second stack could not start

#### Scenario: A volume shared on purpose
- **WHEN** the volume `cache` has `name: pip-cache` and the label `dev.siding.shared: "true"`
- **THEN** lint reports `FixedVolumeName` as info, not as an error

### Requirement: Warnings and info
`lint` SHALL warn about `HostPath` (a bind mount of an absolute path outside the worktree, which
every stack shares), `NetworkMode` (a service siding cannot route) and `NoHealthcheck` (a
service without a healthcheck and without a route path, of which the verdict only knows that it
runs), and report as info `PublishedPort` (ports siding resets) and `NoRoot` (HTTP services but
no `dev.siding.route: "/"`).

#### Scenario: A host path
- **WHEN** `db` mounts `/var/lib/shop:/data`
- **THEN** lint warns `HostPath` for `db`, because every branch's stack would write into the same directory

#### Scenario: A published database port
- **WHEN** `db` publishes `5432:5432`
- **THEN** lint reports `PublishedPort` as info, with the hint that siding resets it and how to reach the database

### Requirement: Missing env files
In a linked worktree, `lint` SHALL warn `MissingEnvFile` for every file that `up` would refuse
to start without (see `stack-lifecycle`, "Env files of the main worktree"), with the `cp`
command as the hint.

#### Scenario: .env missing
- **WHEN** `siding lint` runs in a new worktree that lacks the main worktree's untracked `.env`
- **THEN** it warns `MissingEnvFile` for `.env` with the `cp` command, and exits 0

### Requirement: localhost in the environment
`lint` SHALL warn `LocalhostURL` for every environment variable of a service whose value names
`localhost` or `127.0.0.1` with a port, because behind the proxy that reaches the service's own
container, not the other service or the host. The finding MUST name the service, the variable
and the host and port, and MUST NOT contain the rest of the value.

#### Scenario: An API URL
- **WHEN** `web` has `API_URL=http://localhost:3000/v1`
- **THEN** lint warns `LocalhostURL` for `web` naming `API_URL` and `localhost:3000`, with the hint to use the service name (`http://api:3000`) between services or a path on the stack's address in the browser

#### Scenario: A password in the value
- **WHEN** `api` has `DATABASE_URL=postgres://app:not-a-real-token@127.0.0.1:5432/app`
- **THEN** the finding names `DATABASE_URL` and `127.0.0.1:5432`, and neither the text nor the `LintReport` contains `not-a-real-token`

#### Scenario: Without a port
- **WHEN** `web` has `COOKIE_DOMAIN=localhost`
- **THEN** lint reports nothing for it
