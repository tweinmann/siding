# Spec Delta

## MODIFIED Requirements

### Requirement: A Docker daemon of its own
The devcontainer SHALL run its own Docker daemon (Docker-in-Docker), which also hosts every
stack siding's tests create, and MUST NOT mount or reach the host's Docker socket.
Docker-in-Docker is the only devcontainer setup; there is no variant with the host's socket.

#### Scenario: docker inside the container
- **WHEN** `docker info` runs inside the devcontainer
- **THEN** it reports the container's own host name, not the Docker Desktop VM or the host

#### Scenario: No Docker-outside-of-Docker
- **WHEN** someone looks for a devcontainer configuration of this repository
- **THEN** there is only `.devcontainer/devcontainer.json`, and no configuration mounts `/var/run/docker.sock` from the host

## ADDED Requirements

### Requirement: macOS binaries
`just build-darwin` SHALL build static siding binaries for macOS, `bin/siding-darwin-arm64` and
`bin/siding-darwin-amd64`, in the devcontainer, so siding can be tried on the Mac against
Docker Desktop without installing Go there.

#### Scenario: Try siding on the Mac
- **WHEN** `just build-darwin` has run in the devcontainer
- **THEN** `bin/siding-darwin-arm64 version` runs on an Apple silicon Mac and reports `darwin/arm64`

### Requirement: siding used on the host
What a human creates by running siding on the host (`siding-proxy`, the network `siding`, their
stacks, `~/.siding`) SHALL count as use of the product, not as development footprint:
`hack/nuke.sh` MUST NOT remove it, and it is removed with siding's own commands.

#### Scenario: nuke.sh and a real proxy
- **WHEN** the maintainer ran `siding setup` on the Mac and then runs `hack/nuke.sh`
- **THEN** `siding-proxy` and the network `siding` on the host stay, and `docker compose -p siding down` removes them
