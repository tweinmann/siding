#!/usr/bin/env bash
# End-to-end test of siding's own commands against the devcontainer's inner daemon: setup,
# up --wait, routing by host and path, a Failed verdict with its log tail, ps, down and the
# refusal of down --volumes. It runs the Go tests with the build tag docker:
#   - internal/stack with -compose: docker compose accepts every golden override;
#   - internal/e2e: the commands themselves, with the proxy siding-test-e2e-proxy on the
#     network siding-test-e2e at port 18080 and projects named siding-test-*.
# Every object is removed by its exact name, also when a test fails.
set -euo pipefail
source "$(dirname "$0")/../lib.sh"
require_devcontainer
require_compose 2.24

cd "$(dirname "$0")/../.."
go test -count=1 ./internal/stack -compose
go test -count=1 -tags docker ./internal/e2e

# Prove that nothing is left behind.
left="$(
  docker ps -a --format '{{.Names}}'
  docker network ls --format '{{.Name}}'
  docker volume ls --format '{{.Name}}'
)"
if grep -q "^${SIDING_TEST_PREFIX}" <<<"$left"; then
  die "objects named ${SIDING_TEST_PREFIX}* are left in the inner daemon: $(grep "^${SIDING_TEST_PREFIX}" <<<"$left" | tr '\n' ' ')"
fi
echo "smoke-stacks: ok"
