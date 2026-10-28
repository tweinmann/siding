# Shared helpers for hack/ scripts that run inside the devcontainer. Source, do not execute.

# Every stack a test creates is a Compose project with this prefix, removed by its exact name.
readonly SIDING_TEST_PREFIX=siding-test-

die() {
  echo "error: $*" >&2
  exit 1
}

# Refuse to run anywhere but inside the siding devcontainer, against its own Docker daemon. This
# keeps every script away from the host's Docker daemon, which is shared with other projects.
require_devcontainer() {
  [[ "${SIDING_DEVCONTAINER:-}" == 1 ]] \
    || die "run this inside the siding devcontainer (SIDING_DEVCONTAINER is not set)"
  # The docker-in-docker daemon runs in this container and reports its host name. A daemon
  # reached through a mounted host socket would report the Docker Desktop VM or the host instead.
  local daemon
  daemon="$(docker info --format '{{.Name}}' 2>/dev/null)" \
    || die "no Docker daemon reachable; is the docker-in-docker daemon running?"
  [[ "$daemon" == "$(hostname)" ]] \
    || die "docker talks to daemon '$daemon', not to the one in this container ($(hostname))"
}

# require_compose <major.minor>: stops unless docker compose is at least that version.
require_compose() {
  local have
  have="$(docker compose version --short 2>/dev/null)" || die "docker compose is not installed"
  have="${have#v}"
  [[ "$(printf '%s\n%s\n' "$1" "$have" | sort -V | head -n 1)" == "$1" ]] \
    || die "docker compose $have is too old, need $1 or later"
}

# retry <attempts> <command...>: runs a command until it succeeds, one second apart.
retry() {
  local attempts="$1" i
  shift
  for ((i = 1; i <= attempts; i++)); do
    "$@" && return 0
    sleep 1
  done
  return 1
}
