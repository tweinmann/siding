# Spec Delta

## MODIFIED Requirements

### Requirement: Stack smoke test
`just smoke-stacks` SHALL run siding's own commands end to end against the inner daemon:
`setup`, `up --wait`, routing by host and path, a `Failed` verdict with log tail, `ps`, `down`,
the refusal of `down --volumes`, and with idle stop on a stack that sleeps after its idle
duration, reports `Idle`, and wakes on a request through the proxy. Its proxy, Sablier, network
and projects MUST carry the prefix `siding-test-`, and its proxy MUST listen on 18080.

#### Scenario: Stack smoke test in CI
- **WHEN** a pull request is opened or pushed to, or a commit is pushed to `main`
- **THEN** CI runs `just smoke-stacks` inside the devcontainer, against its own daemon

#### Scenario: It leaves nothing behind
- **WHEN** `just smoke-stacks` ends, passing or failing
- **THEN** no container, network or volume whose name starts with `siding-test-` is left in the inner daemon, and `siding-proxy`, `siding-sablier` and the network `siding` are untouched

#### Scenario: Not part of level 1
- **WHEN** `just test` runs without a Docker daemon
- **THEN** it passes, because the end-to-end tests only build with the `docker` build tag

#### Scenario: Sleep and wake
- **WHEN** the end-to-end test runs a stack with `--idle 10s` and sends no request for 20 seconds
- **THEN** `siding status --json` reports `Idle`, and a request through the proxy is answered by the stack's service
