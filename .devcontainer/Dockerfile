# Development image for siding. Every tool version is pinned here; this file is the single
# source of truth for versions (hack/nuke.sh reads the base image from it). The Docker engine
# comes from the docker-in-docker feature, pinned in devcontainer.json; the feature's own Compose
# would be unpinned, so the Compose plugin is installed here.
FROM mcr.microsoft.com/devcontainers/go:2-1.27-trixie

# Go itself comes with the base image; the build fails if it is not this version.
ARG GO_VERSION=1.27.3
# Debian trixie ships Git 2.47; relative worktree paths need 2.48, so Git is built from source
# into /usr/local, which comes before /usr/bin in PATH and so shadows the base image's Git.
# NO_RUST: Rust is optional until Git 3.0, and building it would need a Rust toolchain here.
ARG GIT_VERSION=2.56.0
ARG JUST_VERSION=1.59.0
# siding needs Compose 2.24 or later for `ports: !reset []`.
ARG COMPOSE_VERSION=5.6.1
ARG GOLANGCI_LINT_VERSION=2.15.0
ARG NODE_VERSION=24.22.2
ARG OPENSPEC_VERSION=1.15.0

# OpenSpec (spec-driven changes, see openspec/) collects usage data unless told not to.
ENV OPENSPEC_TELEMETRY=0 DO_NOT_TRACK=1 NPM_CONFIG_UPDATE_NOTIFIER=false

# Images the smoke test pulls into the inner daemon: Traefik for the proxy (and later for
# siding-proxy), busybox for the demo services.
ENV SIDING_TRAEFIK_IMAGE=traefik:v3.7.16 SIDING_BUSYBOX_IMAGE=busybox:1.37.0

RUN set -eux; \
    go version | grep -q "go${GO_VERSION} " \
      || { echo "base image has $(go version), expected go${GO_VERSION}" >&2; exit 1; }; \
    arch="$(dpkg --print-architecture)"; \
    case "$arch" in \
      arm64) just_arch=aarch64; node_arch=arm64; compose_arch=aarch64 ;; \
      amd64) just_arch=x86_64; node_arch=x64; compose_arch=x86_64 ;; \
      *) echo "unsupported architecture: $arch" >&2; exit 1 ;; \
    esac; \
    apt-get update; \
    apt-get install -y --no-install-recommends jq \
      libcurl4-openssl-dev libexpat1-dev libssl-dev zlib1g-dev; \
    rm -rf /var/lib/apt/lists/*; \
    tmp="$(mktemp -d)"; \
    curl -fsSL "https://www.kernel.org/pub/software/scm/git/git-${GIT_VERSION}.tar.xz" \
      | tar -xJ -C "$tmp"; \
    make -C "$tmp/git-${GIT_VERSION}" -j"$(nproc)" prefix=/usr/local NO_TCLTK=1 NO_GETTEXT=1 NO_RUST=1 \
      INSTALL_SYMLINKS=1 all install >/dev/null; \
    /usr/local/bin/git --version | grep -q "git version ${GIT_VERSION}"; \
    curl -fsSL "https://github.com/casey/just/releases/download/${JUST_VERSION}/just-${JUST_VERSION}-${just_arch}-unknown-linux-musl.tar.gz" \
      | tar -xz -C /usr/local/bin just; \
    mkdir -p /usr/local/lib/docker/cli-plugins; \
    curl -fsSL -o /usr/local/lib/docker/cli-plugins/docker-compose \
      "https://github.com/docker/compose/releases/download/v${COMPOSE_VERSION}/docker-compose-linux-${compose_arch}"; \
    chmod 0755 /usr/local/lib/docker/cli-plugins/docker-compose; \
    curl -fsSL "https://github.com/golangci/golangci-lint/releases/download/v${GOLANGCI_LINT_VERSION}/golangci-lint-${GOLANGCI_LINT_VERSION}-linux-${arch}.tar.gz" \
      | tar -xz -C "$tmp"; \
    mv "$tmp/golangci-lint-${GOLANGCI_LINT_VERSION}-linux-${arch}/golangci-lint" /usr/local/bin/; \
    curl -fsSL "https://nodejs.org/dist/v${NODE_VERSION}/node-v${NODE_VERSION}-linux-${node_arch}.tar.xz" \
      | tar -xJ -C /usr/local --strip-components=1 --exclude='*.md' --exclude=LICENSE; \
    npm install -g "@fission-ai/openspec@${OPENSPEC_VERSION}"; \
    npm cache clean --force; \
    chmod 0755 /usr/local/bin/just /usr/local/bin/golangci-lint; \
    rm -rf "$tmp"

# A named volume copies ownership and mode from the image directory it is mounted over, but only
# if that directory exists in the image. Without this, the cache and config volumes end up owned
# by root and the vscode user cannot write to them.
# The Go directories are world-writable with the sticky bit, like /go in the base image, rather
# than owned by vscode: in CI, devcontainers/ci changes vscode's UID to the runner's, and usermod
# only re-owns the home directory. /go/pkg needs this too, because Go keeps its checksum database
# cache in /go/pkg/sumdb.
RUN mkdir -p /go/pkg/mod /home/vscode/.cache/go-build /home/vscode/.cache/golangci-lint \
      /home/vscode/.claude \
 && chown -R vscode:vscode /home/vscode/.cache /home/vscode/.claude \
 && chown vscode:golang /go/pkg /go/pkg/mod \
 && chmod 1777 /go/pkg /go/pkg/mod
