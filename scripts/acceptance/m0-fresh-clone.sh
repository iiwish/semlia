#!/usr/bin/env bash
set -euo pipefail

readonly ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

: "${SEMLIA_ACCEPTANCE_SOURCE:?set SEMLIA_ACCEPTANCE_SOURCE to a local repository path}"
: "${SEMLIA_ACCEPTANCE_REF:?set SEMLIA_ACCEPTANCE_REF to an exact 40-character commit}"

cd "${ROOT}"
exec env \
  GOENV=off \
  GOFLAGS= \
  GOWORK=off \
  SEMLIA_RUN_FRESH_CLONE=1 \
  go test -v -timeout=45m -run '^TestFreshCloneAcceptance$' ./tests/acceptance/... -count=1
