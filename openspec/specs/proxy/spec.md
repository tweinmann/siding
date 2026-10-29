# proxy Specification

## Purpose
Defines the one shared proxy through which every stack is reached: `siding-proxy`, a Traefik
on the network `siding`, started by `siding setup` and bound to the loopback interface only.

## Requirements

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

### Requirement: Teardown removes the proxy
`siding teardown` SHALL remove `siding-proxy`, its files under `~/.siding/proxy/`, and the
network `siding` unless a container still uses it, and with `--snapshots` also every snapshot of
this machine. It MUST NOT touch any stack: running stacks keep running as plain Compose
projects, and teardown names them. `--dry-run` removes nothing; with `--json` it prints a
`Teardown` document.

#### Scenario: Nothing runs but the proxy
- **WHEN** `siding teardown` runs and no stack exists
- **THEN** `siding-proxy`, the network `siding` and `~/.siding/proxy/` are gone, and `~/.siding/machine-id` stays

#### Scenario: A stack still runs
- **WHEN** `siding teardown` runs while the stack `shop-main` runs
- **THEN** the proxy is removed, the network `siding` stays because `shop-main` uses it, `shop-main` keeps running, and the output names it with the hint `siding down` or `siding gc`

#### Scenario: Dry run
- **WHEN** `siding teardown --dry-run --json` runs
- **THEN** it prints a `Teardown` document with `dryRun: true` listing the proxy and the network, and removes nothing

#### Scenario: No proxy
- **WHEN** `siding teardown` runs on a machine without `siding-proxy`
- **THEN** it reports that there is nothing to remove and exits 0

#### Scenario: Snapshots
- **WHEN** `siding teardown` runs without `--snapshots` and snapshots exist
- **THEN** the snapshots stay and teardown names them; with `--snapshots` they are removed too

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
