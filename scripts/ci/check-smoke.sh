#!/usr/bin/env bash
set -euo pipefail

readonly ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
readonly MAKE_COMMAND="${MAKE:-make}"
started="$(date +%s)"

export COMPOSE_PROJECT_NAME="semlia-smoke-${started}-$$"
export SEMLIA_IMAGE="semlia:smoke-${started}-$$"
free_port() {
  node --input-type=module -e 'import net from "node:net"; const server=net.createServer(); server.listen(0,"127.0.0.1",()=>{console.log(server.address().port);server.close();});'
}
export SEMLIA_HTTP_PORT="$(free_port)"
export SEMLIA_POSTGRES_PORT="$(free_port)"
while [[ "${SEMLIA_POSTGRES_PORT}" == "${SEMLIA_HTTP_PORT}" ]]; do
  export SEMLIA_POSTGRES_PORT="$(free_port)"
done
export SEMLIA_ALLOWED_ORIGINS="http://127.0.0.1:${SEMLIA_HTTP_PORT}"
export SEMLIA_AUTH_MODE=password

cleanup() {
  local status=$?
  trap - EXIT
  ./scripts/dev/compose.sh down --volumes --remove-orphans || status=1
  if docker image inspect "${SEMLIA_IMAGE}" >/dev/null 2>&1; then
    docker image rm "${SEMLIA_IMAGE}" >/dev/null 2>&1 || status=1
  fi
  printf 'Smoke gate completed in %ss.\n' "$(( $(date +%s) - started ))"
  exit "${status}"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

cd "${ROOT}"
"${MAKE_COMMAND}" --no-print-directory web-embed
./scripts/dev/compose.sh up --detach --build --wait --wait-timeout 240
SEMLIA_RUN_SMOKE=1 SEMLIA_SMOKE_URL="http://127.0.0.1:${SEMLIA_HTTP_PORT}" go test -timeout=10m -count=1 ./tests/smoke/...
