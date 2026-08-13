#!/usr/bin/env -S SHELLOPTS= BASHOPTS= BASH_ENV= ENV= /bin/sh

case "$0" in
  */scripts/acceptance/m0-fresh-clone.sh)
    ROOT=${0%/scripts/acceptance/m0-fresh-clone.sh}
    ;;
  scripts/acceptance/m0-fresh-clone.sh)
    ROOT=.
    ;;
  *)
    ROOT=.
    ;;
esac

TRUSTED_PATH=/usr/local/go/bin:/opt/homebrew/bin:/opt/homebrew/sbin:/home/linuxbrew/.linuxbrew/bin:/home/linuxbrew/.linuxbrew/sbin:/opt/local/bin:/opt/local/sbin:/usr/local/bin:/usr/local/sbin:/usr/bin:/bin:/usr/sbin:/sbin:/run/current-system/sw/bin:/nix/var/nix/profiles/default/bin:/snap/bin:/var/lib/snapd/snap/bin
IFS=:
for DIRECTORY in ${PATH:-}; do
  case "${DIRECTORY}/" in
    */../*|*/./*|*//*)
      continue
      ;;
  esac
  case "${DIRECTORY}" in
    /opt/hostedtoolcache/*/bin|/Users/runner/hostedtoolcache/*/bin|/home/runner/setup-pnpm/node_modules/.bin|/Users/runner/setup-pnpm/node_modules/.bin)
      TRUSTED_PATH=${TRUSTED_PATH}:${DIRECTORY}
      ;;
  esac
done

/usr/bin/env -i \
  "HOME=${HOME:-/tmp}" \
  "USER=${USER:-}" \
  "LOGNAME=${LOGNAME:-}" \
  "TMPDIR=${TMPDIR:-/tmp}" \
  "LANG=${LANG:-C}" \
  "LC_ALL=${LC_ALL:-}" \
  "HTTP_PROXY=${HTTP_PROXY:-}" \
  "HTTPS_PROXY=${HTTPS_PROXY:-}" \
  "NO_PROXY=${NO_PROXY:-}" \
  "http_proxy=${http_proxy:-}" \
  "https_proxy=${https_proxy:-}" \
  "no_proxy=${no_proxy:-}" \
  "SSL_CERT_FILE=${SSL_CERT_FILE:-}" \
  "SSL_CERT_DIR=${SSL_CERT_DIR:-}" \
  "PATH=${TRUSTED_PATH}" \
  GOENV=off \
  GOFLAGS= \
  GOWORK=off \
  SEMLIA_RUN_FRESH_CLONE=1 \
  "SEMLIA_ACCEPTANCE_SOURCE=${SEMLIA_ACCEPTANCE_SOURCE:-}" \
  "SEMLIA_ACCEPTANCE_REF=${SEMLIA_ACCEPTANCE_REF:-}" \
  go -C "${ROOT}" test -v -timeout=100m -run '^TestFreshCloneAcceptance$' ./tests/acceptance/... -count=1
