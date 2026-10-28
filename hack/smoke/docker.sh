#!/usr/bin/env bash
# Smoke test of the building blocks siding stands on, against the devcontainer's inner daemon:
#   1. a compose project with two services and healthchecks becomes healthy;
#   2. an override with `ports: !reset []` removes the port the compose file publishes;
#   3. Traefik on the network "siding" routes Host demo.main.localhost to web at / and to api
#      at /api.
# Everything it creates is a Compose project named siding-test-*, removed by exact name.
set -euo pipefail
source "$(dirname "$0")/../lib.sh"
require_devcontainer
require_compose 2.24

testdata="$(cd "$(dirname "$0")/testdata" && pwd)"
demo="${SIDING_TEST_PREFIX}demo"
proxy="${SIDING_TEST_PREFIX}proxy"
# Not 8080: that port belongs to siding-proxy once it exists, and the smoke test must not take it.
export SMOKE_PROXY_PORT="${SMOKE_PROXY_PORT:-18080}"
: "${SIDING_TRAEFIK_IMAGE:?is set in the devcontainer}" "${SIDING_BUSYBOX_IMAGE:?is set in the devcontainer}"

demo_compose=(docker compose --progress quiet -p "$demo" -f "$testdata/compose.yml" -f "$testdata/override.yml")
proxy_compose=(docker compose --progress quiet -p "$proxy" -f "$testdata/proxy.yml")

created_network=0
cleanup() {
  "${demo_compose[@]}" down -v --remove-orphans >/dev/null 2>&1 || true
  "${proxy_compose[@]}" down -v --remove-orphans >/dev/null 2>&1 || true
  if [[ "$created_network" == 1 ]]; then
    docker network rm siding >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT
cleanup

# The network may already exist when a developer runs siding in this container; then it stays.
if ! docker network inspect siding >/dev/null 2>&1; then
  docker network create siding >/dev/null
  created_network=1
fi

echo "== the compose file alone publishes 18081, the override resets it"
docker compose -p "$demo" -f "$testdata/compose.yml" config --format json |
  jq -e '.services.web.ports | length == 1' >/dev/null || die "compose.yml should publish one port"
"${demo_compose[@]}" config --format json |
  jq -e '[.services[].ports // [] | length] | (add // 0) == 0' >/dev/null || die "override did not reset the ports"

echo "== up: two services with healthchecks become healthy"
"${demo_compose[@]}" up -d --wait --wait-timeout 60 >/dev/null || {
  "${demo_compose[@]}" ps -a
  "${demo_compose[@]}" logs --tail 20
  die "the demo project did not become healthy"
}
for service in web api; do
  health="$(docker inspect -f '{{.State.Health.Status}}' "$("${demo_compose[@]}" ps -q "$service")")"
  [[ "$health" == healthy ]] || die "$service is $health, want healthy"
done

echo "== no published port on the running containers"
bindings="$(docker inspect -f '{{json .NetworkSettings.Ports}}' $("${demo_compose[@]}" ps -q) |
  jq -s '[.[] | to_entries[] | .value // [] | length] | add // 0')"
[[ "$bindings" == 0 ]] || die "the demo project publishes $bindings port bindings, want 0"

echo "== Traefik on the network siding routes by host and path"
"${proxy_compose[@]}" up -d --wait --wait-timeout 60 >/dev/null || {
  "${proxy_compose[@]}" logs --tail 20
  die "Traefik did not become healthy"
}
answer() {
  curl -fsS --max-time 2 -H 'Host: demo.main.localhost' "http://127.0.0.1:$SMOKE_PROXY_PORT$1"
}
# Traefik picks up the containers' labels a moment after they start.
expect() {
  [[ "$(answer "$1" 2>/dev/null)" == "$2" ]]
}
retry 30 expect / web || die "Host demo.main.localhost / answered '$(answer / 2>&1 || true)', want web"
retry 30 expect /api api || die "Host demo.main.localhost /api answered '$(answer /api 2>&1 || true)', want api"
status="$(curl -s -o /dev/null -w '%{http_code}' --max-time 2 -H 'Host: other.main.localhost' \
  "http://127.0.0.1:$SMOKE_PROXY_PORT/")"
[[ "$status" == 404 ]] || die "an unknown host answered $status, want 404"

echo "smoke-docker: ok"
