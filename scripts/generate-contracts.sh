#!/usr/bin/env bash
set -euo pipefail

readonly ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly SPEC="${ROOT}/api/openapi/semlia.v1.yaml"
readonly GO_TARGET="${ROOT}/api/gen/go/types.gen.go"
readonly TS_TARGET="${ROOT}/sdk/typescript/src/schema.gen.ts"

mode="${1:---write}"
if [[ "${mode}" != "--write" && "${mode}" != "--check" ]]; then
  printf 'usage: %s [--write|--check]\n' "$0" >&2
  exit 2
fi

tmp_dir="$(mktemp -d "${TMPDIR:-/tmp}/semlia-contracts.XXXXXXXX")"
trap 'find "${tmp_dir}" -depth -delete' EXIT

go_output="${tmp_dir}/types.gen.go"
ts_output="${tmp_dir}/schema.gen.ts"

cd "${ROOT}"
go tool oapi-codegen \
  -generate types,skip-prune \
  -package contract \
  -o "${go_output}" \
  "${SPEC}"
pnpm --dir "${ROOT}/sdk/typescript" exec openapi-typescript \
  "${SPEC}" \
  --output "${ts_output}"

if [[ "${mode}" == "--write" ]]; then
  mkdir -p "$(dirname "${GO_TARGET}")" "$(dirname "${TS_TARGET}")"
  if [[ ! -f "${GO_TARGET}" ]] || ! cmp -s "${go_output}" "${GO_TARGET}"; then
    cp "${go_output}" "${GO_TARGET}"
  fi
  if [[ ! -f "${TS_TARGET}" ]] || ! cmp -s "${ts_output}" "${TS_TARGET}"; then
    cp "${ts_output}" "${TS_TARGET}"
  fi
  printf 'Generated Go and TypeScript contracts.\n'
  exit 0
fi

status=0
check_artifact() {
  local generated="$1"
  local committed="$2"

  if [[ ! -f "${committed}" ]]; then
    printf 'Missing generated artifact: %s\n' "${committed#"${ROOT}/"}" >&2
    status=1
    return
  fi
  if ! cmp -s "${generated}" "${committed}"; then
    printf 'Generated artifact is stale: %s\n' "${committed#"${ROOT}/"}" >&2
    diff -u "${committed}" "${generated}" || true
    status=1
  fi
}

check_artifact "${go_output}" "${GO_TARGET}"
check_artifact "${ts_output}" "${TS_TARGET}"

exit "${status}"
