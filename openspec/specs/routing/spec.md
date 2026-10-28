# routing Specification

## Purpose
Defines how the services of a stack are reached through the proxy: which services count as
HTTP services, how the label `dev.siding.route` maps them to the stack's host and paths, and
how `siding open` finds the address.

## Requirements

### Requirement: HTTP services
A service SHALL be an HTTP service when the user's compose file gives it exactly one distinct
container port, from the targets of `ports` and from `expose`, or when its label
`dev.siding.port` names one. A service with several ports and no `dev.siding.port` is not one.

#### Scenario: One published port
- **WHEN** a service publishes `"3000:3000"`
- **THEN** it is an HTTP service on container port 3000, and the port is no longer published

#### Scenario: Several ports with the label
- **WHEN** a service exposes 3000 and 9229 and carries `dev.siding.port: "3000"`
- **THEN** it is an HTTP service on container port 3000

#### Scenario: No port
- **WHEN** a worker service has neither `ports` nor `expose` nor `dev.siding.port`
- **THEN** it is not routed

### Requirement: Service host
Every HTTP service SHALL answer at `<service>.<repo-slug>.<branch-slug>.localhost`, unless it
carries `dev.siding.route: "false"`.

#### Scenario: An unlabelled HTTP service
- **WHEN** the service `mail` exposes 8025 and has no route label, in the stack `shop` on branch `main`
- **THEN** `mail.shop.main.localhost` reaches it on 8025

#### Scenario: Hidden
- **WHEN** a service carries `dev.siding.route: "false"`
- **THEN** no host or path reaches it through the proxy

### Requirement: Route label
`dev.siding.route: "/"` SHALL make an HTTP service the root of the stack's host, and
`dev.siding.route: "/<path>"` SHALL mount it on the stack's host under that path prefix,
passing the path on unchanged. The longest matching prefix MUST win.

#### Scenario: Root and API
- **WHEN** `web` carries `dev.siding.route: "/"` and `api` carries `dev.siding.route: "/api"` in the stack `shop` on `main`
- **THEN** `shop.main.localhost/` reaches `web`, and `shop.main.localhost/api/users` reaches `api` with the path `/api/users`

#### Scenario: No root
- **WHEN** no service carries `dev.siding.route: "/"`
- **THEN** a path on the stack's host that no route covers answers 404

### Requirement: Invalid routes
`siding up` SHALL fail with `InvalidRoute`, naming the service, and start nothing when a route
label is neither `false` nor an absolute path, when two services claim the same path, or when
a service with a route is not an HTTP service.

#### Scenario: Two services on one path
- **WHEN** `web` and `admin` both carry `dev.siding.route: "/"`
- **THEN** `siding up` fails with `InvalidRoute` naming both

#### Scenario: A route without a port
- **WHEN** a service carries `dev.siding.route: "/api"` but has no container port
- **THEN** `siding up` fails with `InvalidRoute` and a hint to add `expose` or `dev.siding.port`

#### Scenario: A route with several ports
- **WHEN** a service carries `dev.siding.route: "/"` and exposes 3000 and 9229 without `dev.siding.port`
- **THEN** `siding up` fails with `AmbiguousPort` naming the ports

### Requirement: Services that cannot join the proxy network
A service with a `network_mode` SHALL NOT be routed; `up` SHALL warn about it on stderr, and
fail with `InvalidRoute` when it carries a route path.

#### Scenario: network_mode host
- **WHEN** a service uses `network_mode: host` and has no route label
- **THEN** `siding up` starts it unrouted and prints a warning

### Requirement: Open
`siding open [service]` SHALL open the stack's address, or the service's host, in the default
browser; with `--print` or `--json` it SHALL only print it, as text or as an `Address`
document. The address MUST include the proxy's port unless it is 80.

#### Scenario: Print the address
- **WHEN** the proxy listens on 8080 and `siding open --print` runs in the stack `shop` on `main`
- **THEN** it prints `http://shop.main.localhost:8080/`

#### Scenario: Unknown service
- **WHEN** `siding open nope` runs and the stack has no HTTP service `nope`
- **THEN** it fails with `InvalidArguments` and lists the HTTP services
