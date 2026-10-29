# Spec Delta

## MODIFIED Requirements

### Requirement: Setup starts the proxy
`siding setup` SHALL create the network `siding` if it is missing and run the Compose project
`siding` with one container `siding-proxy`: Traefik with the Docker provider on the network
`siding`, restarting unless stopped. It SHALL print the proxy's address, budget and number of
build slots, with `--json` as a `Setup` document.

#### Scenario: First setup
- **WHEN** `siding setup` runs on a machine without the proxy
- **THEN** the network `siding` and the running container `siding-proxy` exist, and it prints `http://*.localhost` as the address

#### Scenario: JSON
- **WHEN** `siding setup --json` runs
- **THEN** it prints a `Setup` document with the proxy's container name, network, image, port, budget in bytes and build slots

#### Scenario: Build slots
- **WHEN** `siding setup --builds 2` runs, and later `siding setup` without `--builds`
- **THEN** `siding-proxy` carries `dev.siding.builds=2` after both

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
