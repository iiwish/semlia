#!/usr/bin/env bash
set -euo pipefail

readonly ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
readonly REPORT_DIR="${ROOT}/build/security"
readonly CACHE_DIR="${ROOT}/build/trivy-cache"
readonly TRIVY_IMAGE="aquasec/trivy:0.73.0@sha256:7cced7cae583819fc7806d4cbc0dbbc7cad18b99f7d3e235192e6da8c091045c"
readonly RELEASE_IMAGE="${SEMLIA_SECURITY_IMAGE:-semlia:security}"
readonly DOCKER_RESOURCE_LABEL="${SEMLIA_DOCKER_RESOURCE_LABEL:-}"

if [[ -n "${DOCKER_RESOURCE_LABEL}" ]]; then
  if [[ ! "${DOCKER_RESOURCE_LABEL}" =~ ^[a-z0-9][a-z0-9_.-]{0,127}=[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$ ]]; then
    printf 'invalid SEMLIA_DOCKER_RESOURCE_LABEL; expected key=value with Docker-safe characters.\n' >&2
    exit 2
  fi
fi

docker_run() {
  if [[ -n "${DOCKER_RESOURCE_LABEL}" ]]; then
    docker run --rm --label "${DOCKER_RESOURCE_LABEL}" "$@"
  else
    docker run --rm "$@"
  fi
}

mkdir -p "${REPORT_DIR}" "${CACHE_DIR}"

if ! docker image inspect "${TRIVY_IMAGE}" >/dev/null 2>&1; then
  docker pull "${TRIVY_IMAGE}"
fi

status=0

printf '\n==> dependency and secret scan\n'
if ! docker_run \
  --user "$(id -u):$(id -g)" \
  --volume "${ROOT}:/workspace:ro" \
  --volume "${CACHE_DIR}:/cache" \
  "${TRIVY_IMAGE}" fs \
  --cache-dir /cache \
  --scanners vuln,secret \
  --include-dev-deps \
  --severity HIGH,CRITICAL \
  --exit-code 1 \
  --skip-version-check \
  --skip-dirs /workspace/.git \
  --skip-dirs /workspace/.semlia \
  --skip-dirs /workspace/build \
  --skip-dirs /workspace/node_modules \
  /workspace 2>&1 | tee "${REPORT_DIR}/filesystem.txt"; then
  status=1
fi

printf '\n==> release container scan\n'
if ! docker_run \
  --volume /var/run/docker.sock:/var/run/docker.sock:ro \
  --volume "${CACHE_DIR}:/cache" \
  "${TRIVY_IMAGE}" image \
  --cache-dir /cache \
  --scanners vuln \
  --severity HIGH,CRITICAL \
  --exit-code 1 \
  --skip-version-check \
  "${RELEASE_IMAGE}" 2>&1 | tee "${REPORT_DIR}/container.txt"; then
  status=1
fi

if (( status != 0 )); then
  printf '\nSecurity gate failed. Reports: %s\n' "${REPORT_DIR}" >&2
  exit "${status}"
fi

printf '\nSecurity gate passed. Reports: %s\n' "${REPORT_DIR}"
