#!/usr/bin/env bash
set -euo pipefail

readonly ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
readonly GO_COMMAND="${GO:-go}"
readonly SEMLIA_VERSION="${SEMLIA_VERSION:-0.0.0-dev}"
readonly SEMLIA_COMMIT="${SEMLIA_COMMIT:-$(git -C "${ROOT}" rev-parse HEAD)}"
readonly TARGET_OS="${GOOS:-$(${GO_COMMAND} env GOOS)}"
readonly TARGET_ARCH="${GOARCH:-$(${GO_COMMAND} env GOARCH)}"
readonly HOST_OS="$(${GO_COMMAND} env GOHOSTOS)"
readonly HOST_ARCH="$(${GO_COMMAND} env GOHOSTARCH)"
readonly OUTPUT_DIR="${SEMLIA_RELEASE_DIR:-${ROOT}/build/release}"

if [[ ! "${SEMLIA_VERSION}" =~ ^[0-9A-Za-z][0-9A-Za-z._+-]*$ ]]; then
  printf 'Invalid SEMLIA_VERSION: %s\n' "${SEMLIA_VERSION}" >&2
  exit 2
fi
if [[ ! "${SEMLIA_COMMIT}" =~ ^[0-9a-f]{40}$ ]]; then
  printf 'SEMLIA_COMMIT must be a complete 40-character Git SHA.\n' >&2
  exit 2
fi

executable="semlia"
if [[ "${TARGET_OS}" == "windows" ]]; then
  executable="semlia.exe"
fi

bundle="semlia-${SEMLIA_VERSION}-${TARGET_OS}-${TARGET_ARCH}"
staging_root="${OUTPUT_DIR}/staging"
staging="${staging_root}/${bundle}"
archive="${OUTPUT_DIR}/${bundle}.tar.gz"
external_sbom="${OUTPUT_DIR}/${bundle}.sbom.cdx.json"

rm -rf "${staging}"
mkdir -p "${staging}/migrations" "${OUTPUT_DIR}"

cd "${ROOT}"
CGO_ENABLED=0 GOOS="${TARGET_OS}" GOARCH="${TARGET_ARCH}" \
  "${GO_COMMAND}" build -trimpath -ldflags="-s -w" -o "${staging}/${executable}" ./cmd/semlia
cp migrations/*.sql "${staging}/migrations/"
cp LICENSE NOTICE SECURITY.md "${staging}/"

CGO_ENABLED=0 GOOS="${HOST_OS}" GOARCH="${HOST_ARCH}" \
  "${GO_COMMAND}" run ./scripts/release/manifest.go manifest \
  -output "${staging}/release.json" \
  -version "${SEMLIA_VERSION}" \
  -commit "${SEMLIA_COMMIT}" \
  -os "${TARGET_OS}" \
  -arch "${TARGET_ARCH}" \
  -executable "${executable}"

"${ROOT}/scripts/release/sbom.sh" \
  "${staging#"${ROOT}/"}" \
  "${staging#"${ROOT}/"}/SBOM.cdx.json"
cp "${staging}/SBOM.cdx.json" "${external_sbom}"

tar -czf "${archive}" -C "${staging_root}" "${bundle}"
tar -tzf "${archive}" "${bundle}/release.json" "${bundle}/SBOM.cdx.json" >/dev/null

cd "${OUTPUT_DIR}"
if command -v sha256sum >/dev/null 2>&1; then
  sha256sum "$(basename "${archive}")" "$(basename "${external_sbom}")" > SHA256SUMS
else
  shasum -a 256 "$(basename "${archive}")" "$(basename "${external_sbom}")" > SHA256SUMS
fi

cd "${ROOT}"
CGO_ENABLED=0 GOOS="${HOST_OS}" GOARCH="${HOST_ARCH}" \
  "${GO_COMMAND}" run ./scripts/release/manifest.go verify -manifest "${staging}/release.json" \
  -sbom "${external_sbom}" \
  -checksums "${OUTPUT_DIR}/SHA256SUMS" \
  -archive "${archive}" \
  -version "${SEMLIA_VERSION}" \
  -commit "${SEMLIA_COMMIT}"

printf 'Release archive: %s\n' "${archive}"
printf 'CycloneDX SBOM: %s\n' "${external_sbom}"
printf 'SHA256SUMS: %s\n' "${OUTPUT_DIR}/SHA256SUMS"
