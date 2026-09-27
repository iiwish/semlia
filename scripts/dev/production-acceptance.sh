#!/usr/bin/env bash
set -euo pipefail
umask 077

if [[ "${1:-}" != "--suite" || ! "${2:-}" =~ ^(desktop|full)$ || $# != 2 ]]; then
  printf '%s\n' 'Usage: production-acceptance.sh --suite desktop|full' >&2
  exit 2
fi
cd "$(dirname "$0")/../.."
repo="$(pwd -P)"
suite="$2"
suite_status=0
clean_env() {
  env -i PATH="${PATH}" HOME="${HOME}" TMPDIR=/tmp LC_ALL=C GOENV=off GOWORK=off GOFLAGS='' GOTOOLCHAIN="${toolchain}" "$@"
}
toolchain=auto
if [[ "${GOTOOLCHAIN:-}" == local ]]; then toolchain=local; fi
if [[ "$suite" == full ]]; then
  if [[ ! -f "$repo/.semlia/full-acceptance-owner" || "$(< "$repo/.semlia/full-acceptance-owner")" != "SP-T007" || -z "${SEMLIA_FULL_DOCKER_ID:-}" ]]; then
    printf '%s\n' 'Full acceptance requires an owned workspace snapshot and explicit isolated Docker ID' >&2
    exit 2
  fi
fi
clean_env go test ./internal/testsupport/productionacceptance
owner_suffix="$(clean_env openssl rand -hex 8)"
[[ "$owner_suffix" =~ ^[0-9a-f]{16}$ ]] || { printf '%s\n' 'Acceptance owner generation failed.' >&2; exit 1; }
SEMLIA_ACCEPTANCE_OWNER="spacc_${owner_suffix}"
SEMLIA_ACCEPTANCE_ROOT="$repo/.semlia/production-acceptance/$SEMLIA_ACCEPTANCE_OWNER"
SEMLIA_ACCEPTANCE_PASSWORD="$(clean_env openssl rand -hex 24)"
[[ "$SEMLIA_ACCEPTANCE_PASSWORD" =~ ^[0-9a-f]{48}$ ]] || { printf '%s\n' 'Acceptance password generation failed.' >&2; exit 1; }
free_port() {
  clean_env node -e 'const s=require("node:net").createServer(); s.listen(0,"127.0.0.1",()=>{console.log(s.address().port);s.close()}); s.on("error",()=>process.exit(1))'
}
valid_port() { [[ "$1" =~ ^[0-9]{1,5}$ ]] && (( 10#$1 > 1023 && 10#$1 <= 65535 )); }
SEMLIA_ACCEPTANCE_DB_PORT="$(free_port)"
http_port="$(free_port)"
valid_port "$SEMLIA_ACCEPTANCE_DB_PORT" && valid_port "$http_port" || { printf '%s\n' 'Acceptance port allocation failed.' >&2; exit 1; }
[[ "$SEMLIA_ACCEPTANCE_DB_PORT" != "$http_port" && "$SEMLIA_ACCEPTANCE_DB_PORT" != 5432 ]]
SEMLIA_ACCEPTANCE_ADDRESS="127.0.0.1:$http_port"
SEMLIA_ACCEPTANCE_BASE_URL="http://$SEMLIA_ACCEPTANCE_ADDRESS"
SEMLIA_ACCEPTANCE_DATABASE_URL="postgres://$SEMLIA_ACCEPTANCE_OWNER:$SEMLIA_ACCEPTANCE_PASSWORD@127.0.0.1:$SEMLIA_ACCEPTANCE_DB_PORT/$SEMLIA_ACCEPTANCE_OWNER?sslmode=disable"
mkdir -p "$(dirname "$SEMLIA_ACCEPTANCE_ROOT")"
mkdir -m 700 "$SEMLIA_ACCEPTANCE_ROOT"
printf '%s' "$SEMLIA_ACCEPTANCE_OWNER" > "$SEMLIA_ACCEPTANCE_ROOT/.owner"
docker_config="$SEMLIA_ACCEPTANCE_ROOT/docker"
mkdir -m 700 "$docker_config"
clean_env node -e 'console.log(JSON.stringify({auths:{"https://index.docker.io/v1/":{}},cliPluginsExtraDirs:[process.env.HOME+"/.docker/cli-plugins"]}))' > "$docker_config/config.json"
docker_host="$(clean_env docker context inspect --format '{{.Endpoints.docker.Host}}')"
[[ "$docker_host" == unix:///* && "$docker_host" != *$'\n'* ]] || { printf '%s\n' 'Acceptance requires a local Unix Docker endpoint.' >&2; exit 1; }
docker_local() { clean_env docker --config "$docker_config" --host "$docker_host" "$@"; }
if [[ "$suite" == full ]]; then
  [[ "$(docker_local info --format '{{.ID}}')" == "$SEMLIA_FULL_DOCKER_ID" ]] || { printf '%s\n' 'Isolated Docker identity mismatch' >&2; exit 2; }
fi
env_file="$SEMLIA_ACCEPTANCE_ROOT/compose.env"
printf 'SEMLIA_ACCEPTANCE_OWNER=%s\nSEMLIA_ACCEPTANCE_PASSWORD=%s\nSEMLIA_ACCEPTANCE_DB_PORT=%s\n' "$SEMLIA_ACCEPTANCE_OWNER" "$SEMLIA_ACCEPTANCE_PASSWORD" "$SEMLIA_ACCEPTANCE_DB_PORT" > "$env_file"
compose() { docker_local compose --project-directory "$repo" --project-name "$SEMLIA_ACCEPTANCE_OWNER" --env-file "$env_file" --file "$repo/compose.production-acceptance.yaml" "$@"; }
acceptance_env=(env -i PATH="${PATH}" HOME="${HOME}" TMPDIR=/tmp LC_ALL=C GOENV=off GOWORK=off GOFLAGS='' GOTOOLCHAIN="${toolchain}"
  SEMLIA_ACCEPTANCE_OWNER="$SEMLIA_ACCEPTANCE_OWNER" SEMLIA_ACCEPTANCE_ROOT="$SEMLIA_ACCEPTANCE_ROOT"
  SEMLIA_ACCEPTANCE_PASSWORD="$SEMLIA_ACCEPTANCE_PASSWORD" SEMLIA_ACCEPTANCE_ADDRESS="$SEMLIA_ACCEPTANCE_ADDRESS"
  SEMLIA_ACCEPTANCE_BASE_URL="$SEMLIA_ACCEPTANCE_BASE_URL" SEMLIA_ACCEPTANCE_DATABASE_URL="$SEMLIA_ACCEPTANCE_DATABASE_URL")
owned_resources() {
  if [[ "$1" == container ]]; then
    docker_local container ls -a -q --filter "label=com.docker.compose.project=$SEMLIA_ACCEPTANCE_OWNER"
  else
    docker_local "$1" ls -q --filter "label=com.docker.compose.project=$SEMLIA_ACCEPTANCE_OWNER"
  fi
}
configured_resources() {
  case "$1" in
    volume) docker_local volume ls --quiet --filter "name=^${SEMLIA_ACCEPTANCE_OWNER}_postgres-data$" ;;
    network) docker_local network ls --quiet --filter "name=^${SEMLIA_ACCEPTANCE_OWNER}_default$" ;;
  esac
}
verify_absent() {
  local kind ids
  for kind in container network volume; do
    ids="$(owned_resources "$kind")" || return 1
    [[ -z "$ids" ]] || return 1
    ids="$(configured_resources "$kind")" || return 1
    [[ -z "$ids" ]] || return 1
  done
}
verify_marker() {
  clean_env node -e 'const fs=require("node:fs");try{const [p,w]=process.argv.slice(1),d=fs.lstatSync(p),f=fs.lstatSync(p+"/.owner");if(!d.isDirectory()||d.isSymbolicLink()||d.uid!==process.getuid()||(d.mode&511)!==448||!f.isFile()||f.isSymbolicLink()||f.uid!==process.getuid()||(f.mode&511)!==384||fs.readFileSync(p+"/.owner","utf8")!==w)process.exit(1);}catch{process.exit(1)}' "$SEMLIA_ACCEPTANCE_ROOT" "$SEMLIA_ACCEPTANCE_OWNER"
}
verify_ownership() {
  verify_marker || return 1
  local kind ids configured id template labels
  for kind in container network volume; do
    ids="$(owned_resources "$kind")" || return 1
    configured="$(configured_resources "$kind")" || return 1
    ids="${ids}"$'\n'"${configured}"
    template='{{ index .Labels "io.semlia.acceptance.owner" }}|{{ index .Labels "com.docker.compose.project" }}'
    if [[ "$kind" == container ]]; then template='{{ index .Config.Labels "io.semlia.acceptance.owner" }}|{{ index .Config.Labels "com.docker.compose.project" }}'; fi
    while IFS= read -r id; do
      [[ -z "$id" ]] && continue
      labels="$(docker_local "$kind" inspect --format "$template" "$id")" || return 1
      [[ "$labels" == "${SEMLIA_ACCEPTANCE_OWNER}|${SEMLIA_ACCEPTANCE_OWNER}" ]] || return 1
    done <<< "$ids"
  done
}
if ! verify_absent; then printf '%s\n' 'Refusing preexisting configured or project-labelled resources.' >&2; exit 1; fi
app_pid=''
started=0
cleanup() {
  result=$?
  trap - EXIT INT TERM
  if ! verify_ownership; then printf '%s\n' 'Acceptance ownership mismatch; cleanup refused.' >&2; exit 1; fi
  if [[ -n "$app_pid" ]]; then
    if kill -0 "$app_pid" 2>/dev/null; then
      parent_pid="$(clean_env ps -p "$app_pid" -o ppid=)" || { printf '%s\n' 'Acceptance process ownership unavailable; cleanup refused.' >&2; exit 1; }
      parent_pid="${parent_pid//[[:space:]]/}"
      [[ "$parent_pid" == "$$" ]] || { printf '%s\n' 'Acceptance process ownership mismatch; cleanup refused.' >&2; exit 1; }
      kill "$app_pid" 2>/dev/null || result=1
    fi
    wait "$app_pid" 2>/dev/null || true
  fi
  if [[ "$started" == 1 ]]; then
    if ! verify_ownership; then printf '%s\n' 'Acceptance ownership changed; teardown refused.' >&2; exit 1; fi
    compose down --volumes --timeout 10 > "$SEMLIA_ACCEPTANCE_ROOT/cleanup.log" 2>&1 || result=1
    verify_absent || result=1
  fi
  printf 'Acceptance evidence: %s\n' "$SEMLIA_ACCEPTANCE_ROOT"
  exit "$result"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
"${acceptance_env[@]}" pnpm --dir web build > "$SEMLIA_ACCEPTANCE_ROOT/build.log" 2>&1
"${acceptance_env[@]}" go build -o "$SEMLIA_ACCEPTANCE_ROOT/server" ./internal/testsupport/productionacceptance
verify_marker && verify_absent || { printf '%s\n' 'Acceptance preflight changed; startup refused.' >&2; exit 1; }
started=1
compose up --detach --wait > "$SEMLIA_ACCEPTANCE_ROOT/compose.log" 2>&1
verify_ownership || { printf '%s\n' 'Acceptance startup ownership mismatch.' >&2; exit 1; }
"${acceptance_env[@]}" "$SEMLIA_ACCEPTANCE_ROOT/server" > "$SEMLIA_ACCEPTANCE_ROOT/server.log" 2>&1 &
app_pid=$!
ready=0
for ((attempt=0; attempt<100; attempt++)); do
  if ! kill -0 "$app_pid" 2>/dev/null; then
    printf '%s\n' 'Acceptance server exited; inspect server.log' >&2
    exit 1
  fi
  if [[ -f "$SEMLIA_ACCEPTANCE_ROOT/bootstrap.json" ]] && clean_env curl --fail --silent "$SEMLIA_ACCEPTANCE_BASE_URL/health/ready" > /dev/null; then ready=1; break; fi
  sleep 0.3
done
[[ "$ready" == 1 ]]
if ! "${acceptance_env[@]}" pnpm --dir web exec playwright test --config playwright.production.config.ts > "$SEMLIA_ACCEPTANCE_ROOT/browser.log" 2>&1; then
  printf '%s\n' 'Production browser suite failed; protected browser.log retained.' >&2
  suite_status=1
fi
if [[ "$suite" == full ]]; then
  "${acceptance_env[@]}" DOCKER_HOST="$docker_host" DOCKER_CONFIG="$docker_config" go test -count=1 -p=1 -timeout=30m ./tests/integration/ingestion ./tests/integration/discovery ./tests/integration/governance ./tests/integration/projection ./tests/integration/catalog ./tests/integration/db > "$SEMLIA_ACCEPTANCE_ROOT/integration.log" 2>&1 || suite_status=1
  "${acceptance_env[@]}" DOCKER_HOST="$docker_host" DOCKER_CONFIG="$docker_config" SEMLIA_RUN_M1_BENCHMARK=1 go test -count=1 -timeout=10m ./tests/performance/catalog -run '^TestCatalog10000Performance$' -v > "$SEMLIA_ACCEPTANCE_ROOT/performance.log" 2>&1 || suite_status=1
fi
exit "$suite_status"
