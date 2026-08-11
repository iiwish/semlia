#!/usr/bin/env bash
set -euo pipefail

readonly ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
readonly MAKE_COMMAND="${MAKE:-make}"
started="$(date +%s)"

cleanup() {
  local status=$?
  trap - EXIT
  "${MAKE_COMMAND}" --no-print-directory dev-down || true
  printf 'Smoke gate completed in %ss.\n' "$(( $(date +%s) - started ))"
  exit "${status}"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

cd "${ROOT}"
"${MAKE_COMMAND}" --no-print-directory dev
"${MAKE_COMMAND}" --no-print-directory smoke
