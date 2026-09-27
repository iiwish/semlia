#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
readonly ROOT
started="$(date +%s)"
umask 077
isolated="$(mktemp -d /tmp/semlia-smoke.XXXXXX)"
readonly isolated
readonly env_file="${isolated}/smoke.env"
readonly owner_file="${isolated}/ownership.yaml"
readonly docker_config="${isolated}/docker"
docker_started=false
docker_host=""

# Compose, Docker and test processes receive no caller runtime overrides.
# HOME is used only to discover the local Docker endpoint/plugins and tool caches.
clean_env() {
  env -i PATH="${PATH}" HOME="${HOME}" TMPDIR=/tmp LC_ALL=C "$@"
}
docker_local() {
  clean_env BUILDX_CONFIG="${isolated}/buildx" docker --config "${docker_config}" --host "${docker_host}" "$@"
}
compose() {
  docker_local compose --project-directory "${ROOT}" --project-name "${project}" \
    --env-file "${env_file}" -f "${ROOT}/compose.yaml" \
    -f "${ROOT}/compose.override.yaml" -f "${owner_file}" "$@"
}

resource_ids() {
  docker_local "$1" ls --quiet --filter "label=com.docker.compose.project=${project}" "${@:2}"
}
configured_ids() {
  local name
  case "$1" in
    volume)
      for name in postgres-data git-content artifact-data; do
        docker_local volume ls --quiet --filter "name=^${project}_${name}$" || return 1
      done ;;
    network) docker_local network ls --quiet --filter "name=^${project}_backend$" ;;
  esac
}
verify_absent() {
  local kind ids
  for kind in container network volume; do
    if [[ "${kind}" == container ]]; then ids="$(resource_ids "${kind}" --all)" || return 1
    else ids="$(resource_ids "${kind}")" || return 1; fi
    [[ -z "${ids}" ]] || return 1
    ids="$(configured_ids "${kind}")" || return 1
    [[ -z "${ids}" ]] || return 1
  done
  ids="$(docker_local image ls --quiet --filter "reference=${image}")" || return 1
  [[ -z "${ids}" ]]
}
verify_resources() {
  local kind ids configured id labels format
  for kind in container network volume; do
    if [[ "${kind}" == container ]]; then ids="$(resource_ids "${kind}" --all)" || return 1
    else ids="$(resource_ids "${kind}")" || return 1; fi
    # Compose can reuse/delete a configured volume by name even without its
    # project label. Check both inventories before permitting `down --volumes`.
    configured="$(configured_ids "${kind}")" || return 1
    ids="${ids}"$'\n'"${configured}"
    if [[ "${kind}" == container ]]; then format='{{ index .Config.Labels "io.semlia.smoke.owner" }}|{{ index .Config.Labels "com.docker.compose.project" }}'
    else format='{{ index .Labels "io.semlia.smoke.owner" }}|{{ index .Labels "com.docker.compose.project" }}'; fi
    while IFS= read -r id; do
      [[ -z "${id}" ]] && continue
      labels="$(docker_local "${kind}" inspect --format "${format}" "${id}")" || return 1
      [[ "${labels}" == "${owner}|${project}" ]] || return 1
    done <<< "${ids}"
  done
  image_ids="$(docker_local image ls --quiet --filter "reference=${image}")" || return 1
  if [[ -n "${image_ids}" ]]; then
    labels="$(docker_local image inspect --format '{{ index .Config.Labels "io.semlia.smoke.owner" }}|{{ index .Config.Labels "io.semlia.smoke.project" }}' "${image}")" || return 1
    [[ "${labels}" == "${owner}|${project}" ]] || return 1
  fi
}
cleanup() {
  local status=$?
  trap - EXIT
  set +e
  if [[ "${docker_started}" == true ]]; then
    if ! verify_resources; then
      printf 'Smoke cleanup refused: resource ownership could not be verified. Credentials retained in %s.\n' "${isolated}" >&2
      exit 1
    fi
    compose down --volumes --remove-orphans || status=1
    if [[ -n "${image_ids}" ]]; then docker_local image rm "${image}" >/dev/null || status=1; fi
  fi
  if [[ "${docker_started}" == false ]] || verify_absent; then
    if [[ "${isolated}" == /tmp/semlia-smoke.* && -d "${isolated}" && ! -L "${isolated}" && -O "${isolated}" ]]; then
      rm -rf -- "${isolated}" || status=1
    else status=1; fi
  else
    status=1
    printf 'Smoke cleanup could not prove resources absent; protected credentials retained in %s.\n' "${isolated}" >&2
  fi
  printf 'Smoke gate completed in %ss.\n' "$(( $(date +%s) - started ))"
  exit "${status}"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

cd "${ROOT}"
docker_host="$(clean_env docker context inspect --format '{{.Endpoints.docker.Host}}')"
if [[ "${docker_host}" != unix:///* || "${docker_host}" == *$'\n'* ]]; then
  printf 'Smoke requires a local Unix-socket Docker context; no resources were changed.\n' >&2
  exit 1
fi
owner="$(clean_env node -e 'console.log(require("node:crypto").randomBytes(16).toString("hex"))')"
if [[ ! "${owner}" =~ ^[0-9a-f]{32}$ ]]; then printf 'Smoke owner generation failed.\n' >&2; exit 1; fi
readonly owner
readonly project="semlia-smoke-${started}-$$-${owner:0:8}"
readonly image="semlia:smoke-${owner}"
mkdir "${docker_config}"
# A nonempty anonymous auth map disables Docker CLI's native credential-store
# auto-detection. It contains no imported credentials, proxies or contexts.
clean_env node -e 'console.log(JSON.stringify({auths:{"https://index.docker.io/v1/":{}},cliPluginsExtraDirs:[process.env.HOME+"/.docker/cli-plugins"]}))' > "${docker_config}/config.json"
if ! verify_absent; then
  printf 'Smoke could not verify an unused project and image; refusing reuse.\n' >&2
  exit 1
fi
free_port() {
  clean_env node --input-type=module -e 'import net from "node:net"; const server=net.createServer(); server.listen(0,"127.0.0.1",()=>{console.log(server.address().port);server.close();});'
}
valid_port() { [[ "$1" =~ ^[0-9]{1,5}$ ]] && (( 10#$1 > 0 && 10#$1 <= 65535 )); }
http_port="$(free_port)"
if ! valid_port "${http_port}"; then printf 'Smoke HTTP port allocation failed.\n' >&2; exit 1; fi
readonly http_port
postgres_port="$(free_port)"
if ! valid_port "${postgres_port}"; then printf 'Smoke PostgreSQL port allocation failed.\n' >&2; exit 1; fi
while [[ "${postgres_port}" == "${http_port}" ]]; do
  postgres_port="$(free_port)"
  if ! valid_port "${postgres_port}"; then printf 'Smoke PostgreSQL port allocation failed.\n' >&2; exit 1; fi
done
{
  printf 'SEMLIA_IMAGE=%s\nSEMLIA_HTTP_PORT=%s\nSEMLIA_POSTGRES_PORT=%s\n' "${image}" "${http_port}" "${postgres_port}"
  printf 'SEMLIA_ALLOWED_ORIGINS=http://127.0.0.1:%s\nSEMLIA_AUTH_MODE=password\nSEMLIA_EXECUTION_SOURCES=\nSEMLIA_EXECUTION_ALLOW_PLAINTEXT=false\nSEMLIA_UAT_LLM_KEY=\n' "${http_port}"
  clean_env node -e 'const {randomBytes}=require("node:crypto"); for(const name of ["SEMLIA_SECRET_KEY","SEMLIA_POSTGRES_PASSWORD"]) console.log(name+"="+randomBytes(32).toString("hex"));'
} > "${env_file}"
{
  printf 'x-smoke-labels: &smoke-labels\n  io.semlia.smoke.owner: "%s"\n  io.semlia.smoke.project: "%s"\nservices:\n' "${owner}" "${project}"
  printf '  postgres:\n    labels: *smoke-labels\n'
  for service in migrate server worker; do
    printf '  %s:\n    labels: *smoke-labels\n    build:\n      labels: *smoke-labels\n' "${service}"
  done
  printf 'networks:\n  backend:\n    labels: *smoke-labels\nvolumes:\n'
  for volume in postgres-data git-content artifact-data; do printf '  %s:\n    labels: *smoke-labels\n' "${volume}"; done
} > "${owner_file}"
clean_env node --input-type=module - "${ROOT}" "${isolated}" "${owner}" "${project}" "${image}" "${docker_host}" "${http_port}" <<'NODE'
import { readFileSync, writeFileSync } from 'node:fs';
import { createHash } from 'node:crypto';
const [root, dir, owner, project, image, dockerHost, port] = process.argv.slice(2);
const digest = path => createHash('sha256').update(readFileSync(path)).digest('hex');
const envFile = `${dir}/smoke.env`, ownershipFile = `${dir}/ownership.yaml`, dockerConfig = `${dir}/docker`;
writeFileSync(`${dir}/context.json`, JSON.stringify({version:1, root, owner, project, image, dockerHost, dockerConfig,
  httpURL:`http://127.0.0.1:${port}`, envFile, ownershipFile, envFileDigest:digest(envFile),
  ownershipFileDigest:digest(ownershipFile), dockerConfigDigest:digest(`${dockerConfig}/config.json`)}), {mode:0o600});
NODE
printf 'Smoke project: %s; image: %s.\n' "${project}" "${image}"
docker_started=true
compose up --detach --build --wait --wait-timeout 240
clean_env GOENV=off GOWORK=off GOFLAGS='' SEMLIA_RUN_SMOKE=1 SEMLIA_SMOKE_CONTEXT="${isolated}/context.json" SEMLIA_SMOKE_URL="http://127.0.0.1:${http_port}" go test -timeout=10m -count=1 ./tests/smoke/...
