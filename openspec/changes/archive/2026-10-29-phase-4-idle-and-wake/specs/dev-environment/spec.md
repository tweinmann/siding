# Spec Delta

## MODIFIED Requirements

### Requirement: Stack smoke test
`just smoke-stacks` SHALL run siding's own commands end to end against the inner daemon:
`setup`, `up --wait`, routing by host and path, a `Failed` verdict with log tail, `ps`, `down`,
the refusal of `down --volumes`, and an `up` that stops an idle stack to make room, which then
reports `Stopped` and starts again with `up --wait`. Its proxy, network and projects MUST carry
the prefix `siding-test-`, and its proxy MUST listen on 18080.

#### Scenario: Stack smoke test in CI
- **WHEN** a pull request is opened or pushed to, or a commit is pushed to `main`
- **THEN** CI runs `just smoke-stacks` inside the devcontainer, against its own daemon

#### Scenario: It leaves nothing behind
- **WHEN** `just smoke-stacks` ends, passing or failing
- **THEN** no container, network or volume whose name starts with `siding-test-` is left in the inner daemon, and `siding-proxy` and the network `siding` are untouched

#### Scenario: Not part of level 1
- **WHEN** `just test` runs without a Docker daemon
- **THEN** it passes, because the end-to-end tests only build with the `docker` build tag

#### Scenario: Making room
- **WHEN** the end-to-end test runs with `--idle 10s` and a small budget, starts one stack, waits 15 seconds without a request, and starts a second that only fits without the first
- **THEN** the first is stopped and reports `Stopped`, the second is Ready, and `up --wait` in the first's worktree after `down` of the second makes it Ready again with its data
