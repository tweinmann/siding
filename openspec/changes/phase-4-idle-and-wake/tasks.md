# Tasks

## 1. Spike: Sablier against the pinned Traefik (gate)

- [x] 1.1 In the inner daemon, run Traefik v3.7.16 with the plugin `v1.4.0` and `sablierapp/sablier:1.10.1` by hand (a `siding-test-spike` project, proxy on 18080) with a two-service stack (`web` routed, `db` with a healthcheck, `web` with `depends_on: condition: service_healthy`, plus a one-off service that exits 0); check and write into design.md: sleep after `sessionDuration`, wake by request with the `blocking` strategy, the start order within the group, what happens to the exited one-off service, what the blocking timeout answers, that a 1-second request opens a session, that a restart with the plugin cache and no network loads the plugin, Traefik's log line when it cannot, and `USR1` reopening the access log; remove the project by name. If the start order is not kept, stop and ask the maintainer (design, Risks)
  - Result: the start order is not kept; the maintainer dropped Sablier (design D1), and this change was revised.

## 2. Foundations

- [x] 2.1 Add the verdict `Stopped` as a constant and the label `dev.siding.idle` to the conventions in CLAUDE.md and `openspec/config.yaml`, and move "wake on request" to "Later" in the roadmap there; verify `openspec validate --all --strict` passes
- [x] 2.2 Add to `internal/docker`: a container's stdout log as lines, and its command and `StartedAt`; add stopping a project by name (`docker compose -p <project> stop`) next to `down`; verify with unit tests of the mappings and the compose arguments

## 3. The proxy's record and the threshold

- [x] 3.1 Add the JSON access log on stdout with only `RouterName` and `StartUTC` and the bounded `json-file` logging to `proxy.ComposeFile` (D3); verify with the updated golden of the proxy's compose file
- [x] 3.2 Add `setup --idle <duration>|off|default` with the label carried forward and the minimum of 10 seconds (D2), and `idle` in `Setup`; verify with fake-based tests per scenario of `idle-stop` "Idle threshold" and the updated `setup.json` golden
- [x] 3.3 Implement `internal/idle` (D4): parse the proxy's log lines, map routers to projects by longest prefix, the last request per stack as the latest of record, stack start and proxy start, and "no record" for a proxy without `--accesslog=true`; verify with table tests (prefix clash `shop-main` / `shop-main-x`, Traefik's own non-JSON lines, never requested, proxy recreated, a v0.3 proxy)

## 4. Making room

- [x] 4.1 Make room in `checkBudget` and for the first in line (D5): candidates, oldest first, shortest prefix that fits, nothing when it would not fit, stop by project name, the stderr line, `--force` and `--idle off` skip it, the hint for a proxy without the record, the log read at most every 30 seconds in the queue; verify with fake-based tests per scenario of `idle-stop` "Idle stacks make room" and `memory-budget` "Stopped stacks use nothing", using the fake clock
- [x] 4.2 Document making room in the README's budget section (threshold, what counts as a request, `--idle`, starting a stopped stack with `siding up`, activity that does not pass the proxy); verify the shown commands run as written in the devcontainer

## 5. The verdict Stopped

- [x] 5.1 Implement `Stopped` in `verdict.Evaluate` (D6); verify with verdict table tests (stopped to make room, stopped by hand, a crash is not a stop, only one-off services stay `Ready`, starting again is `Starting`)
- [x] 5.2 Make `status` evaluate before probing, never probe a `Stopped` stack, end `--wait` on it with exit 2, the `url` and the hint; verify with fake-based tests that no probe is sent and a `status-stopped.json` golden

## 6. Agent skill

- [x] 6.1 Update `internal/cli/skill/SKILL.md` (D7): `Stopped` under exit 2 and how to start the stack again, the `url` from `siding status --json`, never `localhost:<port>`; verify with the skill test for the named commands and absent flags (`--idle` included) and the `skill.json` golden

## 7. End-to-end and integration

- [x] 7.1 Extend `internal/e2e`: `setup --idle 10s` with a small budget, one stack Ready, 15 seconds without a request, a second `up --wait` that stops the first and is Ready; the first reports `Stopped` and keeps its volume; a request to it answers 404; after `down` of the second, `up --wait` in the first's worktree makes it Ready with its data; the proxy's log line holds no path; verify with `just smoke-stacks` and that no `siding-test-*` object is left
- [x] 7.2 Run the flow by hand in the devcontainer and on the Mac (`just build-darwin`): `setup --port 8080 --idle 1m` with a small budget, two worktrees, the second `up` stopping the first, `siding status` and `siding ui` showing it stopped, `siding up --wait` starting it again; record the result in this task
  - Devcontainer (2026-10-29): passed. Two worktrees of one repository, `--idle 1m` and a budget that fits one stack: after a minute without a request, `up` in `feature/two` stopped `demo-main` and said so on stderr; `status` in `main` reported `Stopped` with the hint and exit 2; a request answered 404; `ps` and `siding ui` showed it `stopped` with 0 MiB; `up --wait` made it Ready again. Found: branches of one repository share learned peaks, so they fit far more tightly than their `mem_limit` suggests (correct; the end-to-end test uses two repositories); `status` showed "not answering" for a stopped stack it never probed, now left out. 
  - macOS with Docker Desktop (arm64, 2026-10-29): passed, 15 of 15 checks of `bin/mac-test/run.sh` (setup `--idle 1m` on port 18090, a budget for one stack, two worktrees): `up` in `feature/two` stopped `main` after a minute without a request and said so; `status` `Stopped` with exit 2 and the hint; a request answered 404; `ps` and `siding ui` showed it stopped; `up --wait` made it Ready again with its data; the proxy it started was removed again.
- [x] 7.3 Update the status in CLAUDE.md, the README and `openspec/config.yaml` to v0.4; verify `just test` passes

## Workflow follow-up

- Stop and show the result; wait for the maintainer's approval of Phase 4.
- `/opsx:archive` merges the deltas into `openspec/specs/`.
