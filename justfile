# Development tasks. Every recipe runs inside the devcontainer.
# The one host-side task, removing all siding Docker objects, is hack/nuke.sh: just is not
# installed on the host.

set shell := ["bash", "-euo", "pipefail", "-c"]

# List available recipes
default:
    @just --list

# Level-1 checks, as in CI: formatting, vet, unit tests, lint, specs
test:
    #!/usr/bin/env bash
    set -euo pipefail
    # Only the repository's own files: CI keeps a Go cache under .cache in the workspace.
    unformatted="$(git ls-files -z --cached --others --exclude-standard '*.go' | xargs -0 -r gofmt -l)"
    if [[ -n "$unformatted" ]]; then
      echo "gofmt needed:" >&2
      echo "$unformatted" >&2
      exit 1
    fi
    go vet ./...
    go test -race ./...
    golangci-lint run ./...
    openspec validate --all --strict --no-interactive

# Rewrite golden files from the current code after an intended change
# (every package with golden files; a package without the -update flag would fail on it)
golden:
    go test ./internal/cli ./internal/stack -update

# Build a static siding binary for this container (linux) into bin/
build:
    CGO_ENABLED=0 go build -trimpath -o bin/siding ./cmd/siding

# Inner daemon: a compose project with a healthcheck, ports reset by an override, Traefik routing
smoke-docker:
    hack/smoke/docker.sh

# Inner daemon: siding's own commands end to end (setup, up --wait, routing, verdicts, ps, down)
smoke-stacks:
    hack/smoke/stacks.sh
