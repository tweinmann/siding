# Tasks

## 1. Spike: Sablier against the pinned Traefik (gate)

- [ ] 1.1 In the inner daemon, run Traefik v3.7.16 with the plugin `v1.4.0` and `sablierapp/sablier:1.10.1` by hand (a `siding-test-spike` project, proxy on 18080) with a two-service stack (`web` routed, `db` with a healthcheck, `web` with `depends_on: condition: service_healthy`, plus a one-off service that exits 0); check and write into design.md: sleep after `sessionDuration`, wake by request with the `blocking` strategy, the start order within the group, what happens to the exited one-off service, what the blocking timeout answers, that a 1-second request opens a session (D4), that a restart with the plugin cache and no network loads the plugin, Traefik's log line when it cannot, and `USR1` reopening the access log; remove the project by name. If the start order is not kept, stop and ask the maintainer (design, Risks)

## 2. Foundations

- [ ] 2.1 Pin `sablierapp/sablier:1.10.1` as `SIDING_SABLIER_IMAGE` in `.devcontainer/Dockerfile` next to the Traefik image, and the image and plugin version as constants in `internal/proxy`; verify the image builds and `just test` passes
- [ ] 2.2 Add the error code `PluginUnavailable`, the verdict `Idle` and the `ps` state `idle` as constants; verify with the error-code table test
- [ ] 2.3 Make precise the guardrail "no background service" (D1) and add `dev.siding.idle`, the Sablier labels, `traefik.docker.allownonrunning` and `siding-sablier` to CLAUDE.md and `openspec/config.yaml`; verify `openspec validate --all --strict` passes

## 3. Setup and the proxy project

- [ ] 3.1 Extend `proxy.ComposeFile`: with Sablier, the `sablier` service (D1), the plugin flags, the plugin storage, the access log with only `StartUTC` and `RouterName` (D6); without it, byte for byte the v0.3 file; verify with golden files for both
- [ ] 3.2 Add `setup --idle <duration>|off` with the label carried forward, the minimum of 10 seconds, Sablier while the label is set or idle-labelled containers exist, naming those stacks on `--idle off` (D2), and `idleSeconds` in `Setup`; verify with fake-based tests per scenario of `idle-stop` "Idle setting" and `proxy` "Sablier while idle stop is in use", and the updated `setup.json` golden
- [ ] 3.3 Detect a plugin that does not load after the proxy started and fail with `PluginUnavailable` (D8); verify with a fake-based test feeding the log line found in 1.1
- [ ] 3.4 Remove `siding-sablier` and its files in `teardown`; verify with fake-based tests and the updated `teardown*.json` goldens
- [ ] 3.5 Document `setup --idle` in the README (what sleeps, what wakes it, the access log and its two fields, the plugin download, turning it off); verify the shown commands run as written in the devcontainer

## 4. Stacks that sleep

- [ ] 4.1 Add the Sablier labels and the idle middleware to the override when the proxy carries `dev.siding.idle` (D3); verify with golden overrides with and without idle and `docker compose config` in `just smoke-stacks`
- [ ] 4.2 Send the registering request after `docker compose up -d` (D4); verify with a fake-based test that `up` sends one request with a 1-second timeout to the first routed address and ignores its result, and none for a stack without routes
- [ ] 4.3 Implement `Idle` in `verdict.Evaluate` and the state `idle` in `ps` (D5); verify with verdict table tests (asleep, a crash is not sleep, not idle-enabled, waking is `Starting`) and the `stacklist.json` golden
- [ ] 4.4 Make `status` evaluate before probing, never probe an `Idle` stack, end `--wait` on `Idle` with exit 2 and the hint; verify with fake-based tests that no probe is sent and a `status-idle.json` golden
- [ ] 4.5 Show `idle` in `siding ui`; verify with the page golden

## 5. Making room

- [ ] 5.1 Implement `internal/idle`: read the access log backwards (at most 8 MiB, `access.log.1` too), map router names to projects by longest prefix, last request per project, a missing or broken file is empty; verify with table tests (prefix clash `shop-main` / `shop-main-x`, rotation, broken lines)
- [ ] 5.2 Rotate the access log at 16 MiB in `up` and signal `USR1` to the proxy; verify with a fake-based test of rename and signal
- [ ] 5.3 Make room in `checkBudget` and for the first in line (D7): candidates, oldest first, shortest prefix that fits, stop nothing when it would not fit, `docker compose -p <project> stop`, the stderr line, `--force` skips it; verify with fake-based tests per scenario of `idle-stop` "Idle stacks make room" and `memory-budget` "Sleeping stacks use nothing"
- [ ] 5.4 Document making room in the README's budget section; verify the shown behaviour matches the tests

## 6. Agent skill

- [ ] 6.1 Update `internal/cli/skill/SKILL.md` (D9): `Idle` under exit 2 and how to wake, the `url` from `siding status --json`, never `localhost:<port>`; verify with the skill test for the named commands and absent flags (`--idle` included) and the `skill.json` golden

## 7. End-to-end and integration

- [ ] 7.1 Extend `internal/e2e` with `siding-test-e2e-sablier`: `setup --idle 10s`, `up --wait`, no request for 20 seconds, `status --json` reports `Idle` without waking, a request through the proxy is answered by the service, `ps` shows `idle` before; a second stack that only fits after the idle first is stopped; `teardown` leaves no Sablier; verify with `just smoke-stacks` and that no `siding-test-*` object is left
- [ ] 7.2 Run the flow by hand in the devcontainer and on the Mac (`just build-darwin`): `setup --port 8080 --idle 2m`, a stack sleeping and waking in the browser, `siding ui` showing `idle`, a small budget where `up` stops an idle stack; record the result in this task
- [ ] 7.3 Update the status in CLAUDE.md, the README and `openspec/config.yaml` to v0.4; verify `just test` passes

## Workflow follow-up

- Stop and show the result; wait for the maintainer's approval of Phase 4.
- `/opsx:archive` merges the deltas into `openspec/specs/`.
