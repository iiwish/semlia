#!/usr/bin/env bash
set -euo pipefail

readonly ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
readonly TRIVY_IMAGE="aquasec/trivy:0.73.0@sha256:7cced7cae583819fc7806d4cbc0dbbc7cad18b99f7d3e235192e6da8c091045c"
readonly SOURCE_PATH="${1:-build/semlia}"
readonly OUTPUT_PATH="${2:-build/release/semlia.sbom.cdx.json}"
readonly CACHE_DIR="${ROOT}/build/trivy-cache"

case "${SOURCE_PATH}" in
  /*) printf 'SBOM source must be relative to the repository root.\n' >&2; exit 2 ;;
esac
case "${OUTPUT_PATH}" in
  build/*) ;;
  *) printf 'SBOM output must be below build/.\n' >&2; exit 2 ;;
esac

output_dir="${ROOT}/$(dirname "${OUTPUT_PATH}")"
output_name="$(basename "${OUTPUT_PATH}")"
mkdir -p "${output_dir}" "${CACHE_DIR}"

if ! docker image inspect "${TRIVY_IMAGE}" >/dev/null 2>&1; then
  docker pull "${TRIVY_IMAGE}"
fi

docker run --rm \
  --user "$(id -u):$(id -g)" \
  --volume "${ROOT}:/workspace:ro" \
  --volume "${output_dir}:/reports" \
  --volume "${CACHE_DIR}:/cache" \
  "${TRIVY_IMAGE}" rootfs \
  --cache-dir /cache \
  --skip-version-check \
  --scanners vuln \
  --format cyclonedx \
  --output "/reports/${output_name}" \
  "/workspace/${SOURCE_PATH}"

printf 'CycloneDX SBOM: %s\n' "${OUTPUT_PATH}"
