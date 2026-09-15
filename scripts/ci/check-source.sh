#!/usr/bin/env bash
set -euo pipefail

readonly ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
readonly MAKE_COMMAND="${MAKE:-make}"

run_gate() {
  local label="$1"
  shift
  local started
  started="$(date +%s)"
  printf '\n==> %s\n' "${label}"
  "$@"
  printf '<== %s passed in %ss\n' "${label}" "$(( $(date +%s) - started ))"
}

cd "${ROOT}"
run_gate format "${MAKE_COMMAND}" --no-print-directory format-check
run_gate lint "${MAKE_COMMAND}" --no-print-directory lint
run_gate typecheck "${MAKE_COMMAND}" --no-print-directory typecheck
run_gate tests "${MAKE_COMMAND}" --no-print-directory test
run_gate release-proof node --test scripts/release/proof.test.mjs
run_gate contract-drift "${MAKE_COMMAND}" --no-print-directory contracts-check
run_gate migration-drift "${MAKE_COMMAND}" --no-print-directory db-generate-check
run_gate web-embed-drift "${MAKE_COMMAND}" --no-print-directory web-embed-check
run_gate release-build "${MAKE_COMMAND}" --no-print-directory build
