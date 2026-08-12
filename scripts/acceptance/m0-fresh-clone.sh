#!/usr/bin/env -S SHELLOPTS= BASHOPTS= /bin/sh
set -eu

unset BASH_ENV ENV GNUMAKEFLAGS MAKE MAKEFILES MAKEFLAGS MAKELEVEL MAKEOVERRIDES MFLAGS

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"

: "${SEMLIA_ACCEPTANCE_SOURCE:?set SEMLIA_ACCEPTANCE_SOURCE to a local repository path}"
: "${SEMLIA_ACCEPTANCE_REF:?set SEMLIA_ACCEPTANCE_REF to an exact 40-character commit}"

cd "${ROOT}"
exec env \
  GOENV=off \
  GOFLAGS= \
  GOWORK=off \
  SEMLIA_RUN_FRESH_CLONE=1 \
  go test -v -timeout=45m -run '^TestFreshCloneAcceptance$' ./tests/acceptance/... -count=1
