# dev-environment Specification

## Purpose
Describes how siding is developed and checked without touching the maintainer's machine: a
pinned devcontainer with its own Docker daemon, level-1 checks and a Docker smoke test, the same
in CI, and an exact, removable footprint on the host Docker daemon.

## Requirements

### Requirement: Pinned toolchain in the devcontainer
Every tool used to build, test and run siding in development SHALL come from the devcontainer
image, with its version pinned in `.devcontainer/Dockerfile`, which is the single source of tool
versions: Go, Git (2.48 or later), Node with the `openspec` CLI, `just`, `golangci-lint`, `jq`,
the Docker Compose plugin (which MUST be 2.24 or later), and the images the smoke test pulls.
The Docker engine version is pinned in `.devcontainer/devcontainer.json`.

#### Scenario: A tool version changes
- **WHEN** a tool such as Go, Git, just, golangci-lint, Node or OpenSpec is upgraded
- **THEN** the change is an `ARG` in `.devcontainer/Dockerfile`, and CI picks it up because it runs inside that image

#### Scenario: The base image has another Go
- **WHEN** the base image ships a Go version other than `GO_VERSION`
- **THEN** the image build fails and names both versions

### Requirement: A Docker daemon of its own
The devcontainer SHALL run its own Docker daemon (Docker-in-Docker), which also hosts every
stack siding's tests create, and MUST NOT mount or reach the host's Docker socket.

#### Scenario: docker inside the container
- **WHEN** `docker info` runs inside the devcontainer
- **THEN** it reports the container's own host name, not the Docker Desktop VM or the host

### Requirement: Devcontainer settings
The devcontainer SHALL be named `siding-devcontainer`, set `SIDING_DEVCONTAINER=1`, forward port
8080 inside to 8080 on the host for the proxy and never to another host port, and set
`git config worktree.useRelativePaths true` when it is created, so worktrees work both inside
the container and on the host.

#### Scenario: A worktree made in the container
- **WHEN** `git worktree add` runs inside the devcontainer
- **THEN** the worktree's `.git` file and the repository's back-reference use relative paths

#### Scenario: The host's Git is older than 2.48
- **WHEN** a worktree with relative paths exists, and Git before 2.48 on the host opens the repository
- **THEN** it refuses with "unknown repository extension found: relativeworktrees", so the host needs Git 2.48 or later

#### Scenario: Port 8080 is taken on the host
- **WHEN** the devcontainer opens while another process holds port 8080 on the host
- **THEN** VS Code reports that it cannot forward 8080, instead of forwarding to another port that the addresses siding prints do not name

### Requirement: Guard in the hack scripts
Every script in `hack/` except `hack/nuke.sh` MUST abort unless `SIDING_DEVCONTAINER=1` and the
Docker daemon reports the container's own host name.

#### Scenario: Run outside the devcontainer
- **WHEN** a `hack/` script runs where `SIDING_DEVCONTAINER` is not set
- **THEN** it stops with "run this inside the siding devcontainer" before doing anything

#### Scenario: A host daemon reached through a socket
- **WHEN** `docker` in the container talks to a daemon whose name differs from the container's host name
- **THEN** the script stops and names both

### Requirement: Level-1 checks
`just test` SHALL run gofmt, `go vet`, `go test -race`, `golangci-lint` and
`openspec validate --all --strict`, and needs no Docker daemon. Golden files live under
`testdata/` and are rewritten only by `just golden`.

#### Scenario: Level 1 in CI
- **WHEN** a commit is pushed to any branch
- **THEN** CI runs `just test` inside the devcontainer image

#### Scenario: A golden file differs
- **WHEN** a command's output no longer matches its golden file
- **THEN** the test fails, shows both, and points to `just golden`

### Requirement: Docker smoke test
`just smoke-docker` SHALL prove, against the inner daemon, that a compose project with two
services and healthchecks becomes healthy, that an override with `ports: !reset []` removes the
port the compose file publishes, and that Traefik with the Docker provider on the network
`siding` routes `Host: demo.main.localhost` at `/` and at `/api` to the right services.

#### Scenario: Smoke test in CI
- **WHEN** a pull request is opened or pushed to, or a commit is pushed to `main`
- **THEN** CI runs `just smoke-docker` inside the devcontainer, against its own daemon

#### Scenario: Routing
- **WHEN** the demo project and the proxy are up
- **THEN** `curl -H 'Host: demo.main.localhost' /` answers from `web`, `/api` answers from `api`, and an unknown host answers 404

#### Scenario: Ports reset
- **WHEN** the demo project runs with the override
- **THEN** none of its containers has a published port, although the compose file publishes one

### Requirement: Stack smoke test
`just smoke-stacks` SHALL run siding's own commands end to end against the inner daemon:
`setup`, `up --wait`, routing by host and path, a `Failed` verdict with log tail, `ps`, `down`
and the refusal of `down --volumes`. Its proxy, network and projects MUST carry the prefix
`siding-test-`, and its proxy MUST listen on 18080.

#### Scenario: Stack smoke test in CI
- **WHEN** a pull request is opened or pushed to, or a commit is pushed to `main`
- **THEN** CI runs `just smoke-stacks` inside the devcontainer, against its own daemon

#### Scenario: It leaves nothing behind
- **WHEN** `just smoke-stacks` ends, passing or failing
- **THEN** no container, network or volume whose name starts with `siding-test-` is left in the inner daemon, and `siding-proxy` and the network `siding` are untouched

#### Scenario: Not part of level 1
- **WHEN** `just test` runs without a Docker daemon
- **THEN** it passes, because the end-to-end tests only build with the `docker` build tag

### Requirement: Test stacks
Tests SHALL create stacks only as Compose projects whose name starts with `siding-test-`, and
MUST remove them by their exact project name, also when the test fails. The smoke test's proxy
listens on 18080, not on the proxy port 8080.

#### Scenario: A smoke test fails halfway
- **WHEN** `just smoke-docker` stops with an error
- **THEN** its projects `siding-test-demo` and `siding-test-proxy` are removed with their volumes, and the network `siding` too if the test created it

### Requirement: Footprint on the host Docker daemon
The only objects siding development SHALL create on the host Docker daemon are the devcontainer
`siding-devcontainer`, its image, the base image, and the volumes `siding-gomodcache`,
`siding-gocache`, `siding-claude-config`, `siding-docker` and `siding-containerd`. Every new host
object MUST be added to this requirement, to `hack/nuke.sh` and, for volumes, to
`.devcontainer/devcontainer.json` in the same change.

#### Scenario: The inventory
- **WHEN** `hack/nuke.sh` lists what siding left on the host daemon
- **THEN** it finds at most these objects:

| Object | Name | Removed by `hack/nuke.sh` |
|---|---|---|
| Container | `siding-devcontainer` | yes |
| Image | the devcontainer image (`vsc-siding-…`, generated name) | yes, resolved through the container |
| Image | the base image named in the Dockerfile's `FROM` | only with `--with-shared-images` |
| Volumes | `siding-gomodcache`, `siding-gocache`, `siding-claude-config` | yes |
| Volumes | `siding-docker`, `siding-containerd` (the inner daemon, with every test stack and image) | yes; volumes on these targets are also resolved through the container |

#### Scenario: Everything the tests create
- **WHEN** test stacks, the proxy and their images exist
- **THEN** they live in the inner daemon, inside `siding-docker`, and none of them is an object on the host daemon

#### Scenario: Left in place on purpose
- **WHEN** `hack/nuke.sh` has run
- **THEN** the shared `vscode` volume, the build cache and untagged intermediate images are still there, and the script reports them instead of removing them

### Requirement: Cleanup by exact name
`hack/nuke.sh` SHALL be a POSIX `sh` script that needs only `docker`, MUST refuse to run inside a
container, MUST list what it found and ask before removing it unless `--yes` is given, MUST
remove nothing with `--dry-run`, and MUST remove objects by exact name or by resolving them
through the devcontainer, never by prefix and never with `docker … prune`.

#### Scenario: Run inside the devcontainer
- **WHEN** `hack/nuke.sh` runs where `/.dockerenv` exists or `SIDING_DEVCONTAINER` is set
- **THEN** it stops with "run this in a terminal on the host, not inside a container"

#### Scenario: A foreign container with a similar name
- **WHEN** the host has a container `siding-other` that belongs to another project
- **THEN** `hack/nuke.sh` neither lists nor removes it

#### Scenario: Dry run
- **WHEN** `hack/nuke.sh --dry-run` runs
- **THEN** it lists what it would remove and removes nothing

### Requirement: No secrets in the repository
The repository, golden files and logs MUST NOT contain real secrets; golden files MAY contain
obvious fakes such as `not-a-real-token`.

#### Scenario: A golden file needs a token
- **WHEN** a test's output contains a token
- **THEN** the test uses an obvious fake, and the golden file holds only that fake
