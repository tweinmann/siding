# Spec Delta

## MODIFIED Requirements

### Requirement: Setup starts the proxy
`siding setup` SHALL create the network `siding` if it is missing and run the Compose project
`siding` with one container `siding-proxy`: Traefik with the Docker provider on the network
`siding`, restarting unless stopped. It SHALL print the proxy's address, budget, number of
build slots and idle threshold, with `--json` as a `Setup` document.

#### Scenario: First setup
- **WHEN** `siding setup` runs on a machine without the proxy
- **THEN** the network `siding` and the running container `siding-proxy` exist, and it prints `http://*.localhost` as the address

#### Scenario: JSON
- **WHEN** `siding setup --json` runs
- **THEN** it prints a `Setup` document with the proxy's container name, network, image, port, budget in bytes, build slots and `idle` (`5m0s`, another duration, or `off`)

#### Scenario: Build slots
- **WHEN** `siding setup --builds 2` runs, and later `siding setup` without `--builds`
- **THEN** `siding-proxy` carries `dev.siding.builds=2` after both

## ADDED Requirements

### Requirement: Record of requests
The proxy SHALL record, for every request it routes, only the router and the time, in its
container log, which Docker keeps at a bounded size. The record MUST NOT hold the URL, query,
headers, cookies or client address of a request. It is losable: a recreated proxy starts an
empty record.

#### Scenario: What a request leaves
- **WHEN** a browser requests `http://shop.main.localhost/account?token=not-a-real-token` with a cookie
- **THEN** the proxy's log holds one line with the router of `shop-main` and the time, and neither the path, the query nor the cookie

#### Scenario: Bounded
- **WHEN** the proxy has routed many requests
- **THEN** its container log stays below 30 MiB

#### Scenario: A proxy from siding v0.3
- **WHEN** `siding-proxy` was created by siding v0.3 and `siding setup` of v0.4 runs
- **THEN** the proxy is recreated with the record, on the port it had, and running stacks keep running
