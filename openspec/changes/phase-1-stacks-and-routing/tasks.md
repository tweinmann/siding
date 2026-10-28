# Tasks

## 1. Foundations

- [ ] 1.1 Add `Git`, `Compose`, `Docker`, `Probe`, `Browser`, `Clock`, `Home`, `Env` and `Proxy` to `cli.Options` as interfaces or values (D14), with real implementations wired in `cmd/siding/main.go` and fakes in a test helper; verify `just test` passes and no package-level variable is used as a seam
- [ ] 1.2 Add every error code of the spec `cli-contract` with a hint field on `Error`, printed as a second stderr line in text mode and as `hint` in the `Error` document; verify with a table test per code and the updated `error.json` golden
- [ ] 1.3 Extend the exit codes with 2 (TimedOut/Starting) and 3 (NoCapacity, unused) and give `Execute` a way to return a verdict's exit code; verify with a unit test of `Execute`

## 2. Stack identity

- [ ] 2.1 Implement `internal/git`: top level, common dir, current branch (error on detached HEAD), dirty check, merged-into-default-branch check; verify with table tests against temporary repositories (`git init` in `t.TempDir()`), including linked worktrees, detached HEAD, a missing default branch and `.siding/` being ignored
- [ ] 2.2 Implement slugs, project name and hosts in `internal/stack`; verify with a table test covering slashes, case, runs of symbols, leading and trailing symbols, 40/41/60 characters and the hash suffix
- [ ] 2.3 Implement the machine id (D6) with the os id source and `~/.siding/machine-id` from `Options.Home`; verify that two calls agree, that the raw id never appears, and that the fallback file is created once
- [ ] 2.4 Implement the label set (`dev.siding.repo`, `branch`, `worktree`, `machine`, optional `agent` from `--agent` or `SIDING_AGENT`); verify with a unit test

## 3. Compose config, routing and the override

- [ ] 3.1 Implement `internal/compose`: find the compose file (`--file` or Compose's order), run `config --format json`, parse services, ports, `expose`, labels, networks, `network_mode`, healthchecks, named and external volumes and networks, `env_file`; check Compose >= 2.24; verify with golden inputs captured from the devcontainer's Compose in `internal/compose/testdata/`
- [ ] 3.2 Implement HTTP detection and the routing table (service hosts, route paths, `false`, `dev.siding.port`) with the `InvalidRoute` and `AmbiguousPort` errors and the `network_mode` warning; verify with a table test per scenario of the spec `routing`
- [ ] 3.3 Generate `.siding/override.yml` (D7) including `ports: !reset []`, networks, Traefik routers, the `X-Siding-Service` headers middleware and labels on own networks and volumes; verify with golden files in `internal/stack/testdata/override/` (`just golden`) and by checking that `docker compose config` accepts each golden override in the devcontainer
- [ ] 3.4 Add `dev.siding.port` to the label conventions in CLAUDE.md and `openspec/config.yaml`; verify `openspec validate --all --strict` passes

## 4. Docker API and the proxy

- [ ] 4.1 Add `github.com/moby/moby/client` and implement `internal/docker`: containers and their state by project label, all siding stacks, published ports of a container, logs tail, one-shot memory stats, network and volume lists by label; `DockerUnavailable` when the daemon is unreachable; verify with unit tests of the mapping from API types to siding's types
- [ ] 4.2 Implement `internal/proxy` and `siding setup [--port] [--json]` (D4): embedded compose file, network `siding`, `127.0.0.1:<port>`, the `dev.siding.repo` constraint, idempotence, recreate on port or image change, `PortInUse`; verify with fake-based tests of the decisions and a `setup.json` golden
- [ ] 4.3 Document `siding setup` in the README (port 80, `--port`, `--port 8080` in the devcontainer, `*.localhost` resolution); verify the commands in the README match `siding setup --help`

## 5. Up

- [ ] 5.1 Implement `siding up [--file] [--repo] [--agent] [--json] [-- args]`: identity, `ProxyNotRunning`, `SlugCollision`, write `.siding/override.yml` and `.siding/.gitignore`, run Compose with `--remove-orphans` and the passed-through arguments, `ComposeFailed`, Compose progress on stderr; verify with fake-based tests per scenario of the spec `stack-lifecycle` (Up, Override, never edited) and a `stack.json` golden
- [ ] 5.2 Document `siding up` in the README, including that published ports are reset and how to reach a database (`docker compose exec`); verify the example runs as written in the devcontainer

## 6. Verdict and status

- [ ] 6.1 Implement the probe (D9) against `Options.Probe`: `Host` header, no redirects, `X-Siding-Service` and the 502/503/504 rule; verify with an `httptest` server for each case
- [ ] 6.2 Implement `internal/verdict` (D10): Ready, Starting, Failed with `Exited`, `Unhealthy`, `Restarting`, `Missing`, TimedOut with `NotAnswering`, hints, order of evaluation, first failing service in compose order; verify with a table test per scenario of the spec `stack-status`
- [ ] 6.3 Implement `siding status [--wait] [--timeout] [--json]` with polling through `Options.Clock`, the 20-line log tail, `StackNotFound`, exit codes 0/1/2; verify with fake-based tests including a timeout without real waiting, and `status-ready.json`, `status-failed.json` and `status-timedout.json` goldens
- [ ] 6.4 Add `siding up --wait [--timeout]` ending like `status --wait`; verify with a fake-based test that one `Status` document is printed and the exit code follows the verdict

## 7. ps

- [ ] 7.1 Implement `siding ps [--json]`: all stacks by `dev.siding.repo`, state `running`/`partial`/`stopped`, address with the proxy port, memory per stack and in total, `worktreeMissing`; verify with fake-based tests and `stacklist.json` and `stacklist-empty.json` goldens
- [ ] 7.2 Document `siding ps` and `siding status` in the README; verify the shown output matches the text golden of the tests

## 8. Logs, open and down

- [ ] 8.1 Implement redaction (D12): parse `.env` and `env_file`s, replace values of 4+ characters, longest first; verify with a table test using `not-a-real-token` and quoted, commented and short values
- [ ] 8.2 Implement `siding logs [service…] [-f] [--tail] [--json]` (Compose for text, the Docker API and redaction for the `Logs` document, `InvalidArguments` for `--json -f`); apply redaction to the status log tail too; verify with fake-based tests and a `logs.json` golden
- [ ] 8.3 Implement `siding open [service] [--print] [--json]` with `Options.Browser` (`open` on macOS, `$BROWSER` or `xdg-open` on Linux) and the `Address` document; verify with fake-based tests for port 80, 8080 and an unknown service, and an `address.json` golden
- [ ] 8.4 Implement `siding down [--volumes] [--force] [--dry-run] [--json]` (D11) by project name, the `UnmergedBranch` and `DirtyWorktree` guard for `--volumes`, `StackNotFound`, the `Down` document; verify with fake-based tests per scenario and `down.json` and `down-dryrun.json` goldens
- [ ] 8.5 Make the guardrail "never lose work" precise in CLAUDE.md and `openspec/config.yaml` (data and worktrees are guarded, containers are not) and document `logs`, `open` and `down` in the README; verify `just test` passes

## 9. Agent skill

- [ ] 9.1 Write the skill text (`internal/cli/skill/SKILL.md`) and `siding skill [--json]` with the `Skill` document; verify with a golden of the printed text, a test that it names `siding up --wait --json`, `status --json`, `logs --json` and exit codes 0/1/2 and none of `--force`, `--volumes`, `share`, and a `skill.json` golden
- [ ] 9.2 Document installing the skill in the README; verify the shown command produces the file in a temporary directory

## 10. End-to-end and CI

- [ ] 10.1 Write `internal/e2e` with `//go:build docker` (D14): proxy `siding-test-proxy` on network `siding-test` at 18080, temporary repositories with `--repo siding-test-…`, a two-service app with `/` and `/api`, cleanup by exact name in `t.Cleanup`; cases: setup idempotent, `up --wait` Ready, Host and path routing, the service host, `X-Siding-Service` present on app responses and the 502 rule, a failing service giving `Failed` with log tail and a redacted `.env` value, `ps`, `down` keeping volumes, `down --volumes` refused when dirty and unmerged and allowed with `--force`; verify with `just smoke-stacks` in the devcontainer
- [ ] 10.2 Add the recipe `smoke-stacks` (`hack/smoke/stacks.sh` with `require_devcontainer`, running `go test -tags docker ./internal/e2e`) to the justfile and the CI smoke job, and verify afterwards that no `siding-test-*` object is left in the inner daemon and that `just test` still passes without Docker
- [ ] 10.3 Update the dev-environment table in CLAUDE.md and the README with `just smoke-stacks` and `siding setup --port 8080` for the devcontainer; verify the commands run as written
- [ ] 10.4 Run the whole flow by hand in the devcontainer: `just build`, `bin/siding setup --port 8080`, `up --wait` in a sample project, open `http://<host>.localhost:8080` from the host browser, `ps`, `down`; record the result in the phase summary
- [ ] 10.5 Update the status in CLAUDE.md, the README's pre-alpha note and the status in `openspec/config.yaml` to v0.1; verify `just test` passes

## Workflow follow-up

- Stop and show the result; wait for the maintainer's approval of Phase 1.
- `/opsx:archive` merges the deltas into `openspec/specs/`.
- Tag `v0.1.0` after archive, if the maintainer wants a release.
