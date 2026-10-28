# Spec Delta

## MODIFIED Requirements

### Requirement: Setup starts the proxy
`siding setup` SHALL create the network `siding` if it is missing and run the Compose project
`siding` with one container `siding-proxy`: Traefik with the Docker provider on the network
`siding`, restarting unless stopped. It SHALL print the proxy's address and budget, with
`--json` as a `Setup` document.

#### Scenario: First setup
- **WHEN** `siding setup` runs on a machine without the proxy
- **THEN** the network `siding` and the running container `siding-proxy` exist, and it prints `http://*.localhost` as the address

#### Scenario: JSON
- **WHEN** `siding setup --json` runs
- **THEN** it prints a `Setup` document with the proxy's container name, network, image, port and budget in bytes

### Requirement: Setup is idempotent
`siding setup` SHALL change nothing when the proxy already runs as requested, and SHALL
recreate only the proxy container when its configuration differs (port, image, budget, or the
proxy configuration of a newer siding). Running stacks MUST keep running and be routed again
once the proxy is back.

#### Scenario: Second run
- **WHEN** `siding setup` runs twice
- **THEN** the second run reports the proxy as already running and recreates nothing

#### Scenario: Port change with running stacks
- **WHEN** a stack runs and `siding setup --port 8080` follows `siding setup`
- **THEN** the stack's containers are not restarted and it answers on port 8080

#### Scenario: A proxy from siding v0.1
- **WHEN** `siding-proxy` was created by siding v0.1 and `siding setup` of v0.2 runs
- **THEN** the proxy is recreated with the route to the overview, on the port it had

## ADDED Requirements

### Requirement: Route to the overview
The proxy SHALL route `siding.localhost` to `siding ui` while it runs, through a route file
that `siding ui` writes into `~/.siding/proxy/<project>/routes/` and removes when it stops. The
proxy SHALL reach the machine siding runs on as `host.docker.internal`.

#### Scenario: A route file
- **WHEN** `siding ui` has written its route file
- **THEN** the proxy sends `Host: siding.localhost` to `siding ui` within a few seconds, without a restart

#### Scenario: A route left behind
- **WHEN** `siding ui` was killed and its route file is still there
- **THEN** the proxy answers 502 for `siding.localhost`, and the next `siding ui` replaces the file
