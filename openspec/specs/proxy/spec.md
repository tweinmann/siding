# proxy Specification

## Purpose
Defines the one shared proxy through which every stack is reached: `siding-proxy`, a Traefik
on the network `siding`, started by `siding setup` and bound to the loopback interface only.

## Requirements

### Requirement: Setup starts the proxy
`siding setup` SHALL create the network `siding` if it is missing and run the Compose project
`siding` with one container `siding-proxy`: Traefik with the Docker provider on the network
`siding`, restarting unless stopped. It SHALL print the proxy's address, with `--json` as a
`Setup` document.

#### Scenario: First setup
- **WHEN** `siding setup` runs on a machine without the proxy
- **THEN** the network `siding` and the running container `siding-proxy` exist, and it prints `http://*.localhost` as the address

#### Scenario: JSON
- **WHEN** `siding setup --json` runs
- **THEN** it prints a `Setup` document with the proxy's container name, network, image and port

### Requirement: Loopback port
The proxy SHALL publish exactly one port, on `127.0.0.1` only, `80` unless `--port <n>` names
another. siding MUST read the proxy's port from the running container, not from a file.

#### Scenario: Default port
- **WHEN** `siding setup` has run
- **THEN** `siding-proxy` publishes `127.0.0.1:80` and no other port, and nothing listens for it on other interfaces

#### Scenario: Another port
- **WHEN** `siding setup --port 8080` has run and `siding open` runs later
- **THEN** the address it opens is `http://<host>:8080`

#### Scenario: The port is taken
- **WHEN** another process holds `127.0.0.1:80` and `siding setup` runs
- **THEN** it fails with `PortInUse` and a hint to use `--port`

### Requirement: Setup is idempotent
`siding setup` SHALL change nothing when the proxy already runs as requested, and SHALL
recreate only the proxy container when its port or image differs. Running stacks MUST keep
running and be routed again once the proxy is back.

#### Scenario: Second run
- **WHEN** `siding setup` runs twice
- **THEN** the second run reports the proxy as already running and recreates nothing

#### Scenario: Port change with running stacks
- **WHEN** a stack runs and `siding setup --port 8080` follows `siding setup`
- **THEN** the stack's containers are not restarted and it answers on port 8080

### Requirement: Only siding stacks are routed
The proxy MUST route only containers that carry `dev.siding.repo` and enable Traefik, so that
other containers with Traefik labels on the same daemon are never exposed through it.

#### Scenario: A foreign Traefik label
- **WHEN** a container without siding labels carries `traefik.enable=true` and a Host rule
- **THEN** the proxy answers 404 for that host

### Requirement: Stacks need the proxy
`up` and `status` SHALL fail with `ProxyNotRunning` and the hint `siding setup` when
`siding-proxy` is not running.

#### Scenario: up before setup
- **WHEN** `siding up` runs on a machine without the proxy
- **THEN** it fails with `ProxyNotRunning` before starting any container
