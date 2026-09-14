#!/usr/bin/env bash
set -euo pipefail

if [[ "${1:-}" != "--suite" || ! "${2:-}" =~ ^(desktop|full)$ || $# != 2 ]]; then
  printf '%s\n' 'Usage: production-acceptance.sh --suite desktop|full' >&2
  exit 2
fi
cd "$(dirname "$0")/../.."
repo="$(pwd -P)"
suite="$2"
suite_status=0
if [[ "$suite" == full ]]; then
  if [[ ! -f "$repo/.semlia/full-acceptance-owner" || "$(< "$repo/.semlia/full-acceptance-owner")" != "SP-T007" || -z "${SEMLIA_FULL_DOCKER_ID:-}" ]]; then
    printf '%s\n' 'Full acceptance requires an owned workspace snapshot and explicit isolated Docker ID' >&2
    exit 2
  fi
  [[ "$(docker info --format '{{.ID}}')" == "$SEMLIA_FULL_DOCKER_ID" ]] || { printf '%s\n' 'Isolated Docker identity mismatch' >&2; exit 2; }
fi
go test ./internal/testsupport/productionacceptance
export SEMLIA_ACCEPTANCE_OWNER="spacc_$(openssl rand -hex 8)"
export SEMLIA_ACCEPTANCE_ROOT="$repo/.semlia/production-acceptance/$SEMLIA_ACCEPTANCE_OWNER"
export SEMLIA_ACCEPTANCE_PASSWORD="$(openssl rand -hex 24)"
free_port() {
  node -e 'const s=require("node:net").createServer(); s.listen(0,"127.0.0.1",()=>{console.log(s.address().port);s.close()}); s.on("error",()=>process.exit(1))'
}
export SEMLIA_ACCEPTANCE_DB_PORT="$(free_port)"
http_port="$(free_port)"
[[ "$SEMLIA_ACCEPTANCE_DB_PORT" != "$http_port" && "$SEMLIA_ACCEPTANCE_DB_PORT" != 5432 ]]
export SEMLIA_ACCEPTANCE_ADDRESS="127.0.0.1:$http_port"
export SEMLIA_ACCEPTANCE_BASE_URL="http://$SEMLIA_ACCEPTANCE_ADDRESS"
export SEMLIA_ACCEPTANCE_DATABASE_URL="postgres://$SEMLIA_ACCEPTANCE_OWNER:$SEMLIA_ACCEPTANCE_PASSWORD@127.0.0.1:$SEMLIA_ACCEPTANCE_DB_PORT/$SEMLIA_ACCEPTANCE_OWNER?sslmode=disable"
compose=(docker compose --project-name "$SEMLIA_ACCEPTANCE_OWNER" --file "$repo/compose.production-acceptance.yaml")
owned_resources() {
  if [[ "$1" == container ]]; then
    docker container ls -a -q --filter "label=com.docker.compose.project=$SEMLIA_ACCEPTANCE_OWNER"
  else
    docker "$1" ls -q --filter "label=com.docker.compose.project=$SEMLIA_ACCEPTANCE_OWNER"
  fi
}
for kind in container network volume; do
  if [[ -n "$(owned_resources "$kind")" ]]; then
    printf 'Refusing preexisting %s resources\n' "$kind" >&2
    exit 1
  fi
done
mkdir -p "$(dirname "$SEMLIA_ACCEPTANCE_ROOT")"
mkdir -m 700 "$SEMLIA_ACCEPTANCE_ROOT"
printf '%s' "$SEMLIA_ACCEPTANCE_OWNER" > "$SEMLIA_ACCEPTANCE_ROOT/.owner"
app_pid=''
started=0
cleanup() {
  result=$?
  trap - EXIT INT TERM
  if [[ -n "$app_pid" ]]; then
    kill "$app_pid" 2>/dev/null || true
    wait "$app_pid" 2>/dev/null || true
  fi
  if [[ "$started" == 1 ]]; then
    for kind in container network volume; do
      for id in $(owned_resources "$kind"); do
        template='{{ index .Labels "io.semlia.acceptance.owner" }}'
        if [[ "$kind" == container ]]; then template='{{ index .Config.Labels "io.semlia.acceptance.owner" }}'; fi
        if [[ "$(docker "$kind" inspect --format "$template" "$id")" != "$SEMLIA_ACCEPTANCE_OWNER" ]]; then
          printf 'Cleanup refused: mismatched %s owner\n' "$kind" >&2
          exit 1
        fi
      done
    done
    "${compose[@]}" down --volumes --timeout 10 > "$SEMLIA_ACCEPTANCE_ROOT/cleanup.log" 2>&1 || result=1
  fi
  printf 'Acceptance evidence: %s\n' "$SEMLIA_ACCEPTANCE_ROOT"
  exit "$result"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
pnpm --dir web build > "$SEMLIA_ACCEPTANCE_ROOT/build.log" 2>&1
go build -o "$SEMLIA_ACCEPTANCE_ROOT/server" ./internal/testsupport/productionacceptance
started=1
"${compose[@]}" up --detach --wait > "$SEMLIA_ACCEPTANCE_ROOT/compose.log" 2>&1
"$SEMLIA_ACCEPTANCE_ROOT/server" > "$SEMLIA_ACCEPTANCE_ROOT/server.log" 2>&1 &
app_pid=$!
ready=0
for ((attempt=0; attempt<100; attempt++)); do
  if ! kill -0 "$app_pid" 2>/dev/null; then
    printf '%s\n' 'Acceptance server exited; inspect server.log' >&2
    exit 1
  fi
  if [[ -f "$SEMLIA_ACCEPTANCE_ROOT/bootstrap.json" ]] && curl --fail --silent "$SEMLIA_ACCEPTANCE_BASE_URL/health/ready" > /dev/null; then ready=1; break; fi
  sleep 0.3
done
[[ "$ready" == 1 ]]
pnpm --dir web exec playwright test --config playwright.production.config.ts || suite_status=1
if [[ "$suite" == full ]]; then
  go test -count=1 -p=1 -timeout=30m ./tests/integration/ingestion ./tests/integration/discovery ./tests/integration/governance ./tests/integration/projection ./tests/integration/catalog ./tests/integration/db > "$SEMLIA_ACCEPTANCE_ROOT/integration.log" 2>&1 || suite_status=1
  SEMLIA_RUN_M1_BENCHMARK=1 go test -count=1 -timeout=10m ./tests/performance/catalog -run '^TestCatalog10000Performance$' -v > "$SEMLIA_ACCEPTANCE_ROOT/performance.log" 2>&1 || suite_status=1
fi
exit "$suite_status"
