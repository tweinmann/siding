# Spec Delta

## ADDED Requirements

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
