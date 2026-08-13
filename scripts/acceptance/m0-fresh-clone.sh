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

PINNED_GO_VERSION=1.26.5
PINNED_GO_BOOTSTRAP_VERSION=1.26.3
TRUSTED_PATH=/usr/local/go/bin:/opt/homebrew/bin:/opt/homebrew/sbin:/home/linuxbrew/.linuxbrew/bin:/home/linuxbrew/.linuxbrew/sbin:/opt/local/bin:/opt/local/sbin:/usr/local/bin:/usr/local/sbin:/usr/bin:/bin:/usr/sbin:/sbin:/run/current-system/sw/bin:/nix/var/nix/profiles/default/bin:/snap/bin:/var/lib/snapd/snap/bin
TRUSTED_PATH=${TRUSTED_PATH}:/opt/hostedtoolcache/go/1.26.5/x64/bin:/opt/hostedtoolcache/go/1.26.5/arm64/bin
TRUSTED_PATH=${TRUSTED_PATH}:/Users/runner/hostedtoolcache/go/1.26.5/x64/bin:/Users/runner/hostedtoolcache/go/1.26.5/arm64/bin
TRUSTED_PATH=${TRUSTED_PATH}:/opt/hostedtoolcache/node/24.15.0/x64/bin:/opt/hostedtoolcache/node/24.15.0/arm64/bin
TRUSTED_PATH=${TRUSTED_PATH}:/Users/runner/hostedtoolcache/node/24.15.0/x64/bin:/Users/runner/hostedtoolcache/node/24.15.0/arm64/bin
TRUSTED_PATH=${TRUSTED_PATH}:/home/runner/setup-pnpm/node_modules/.bin:/Users/runner/setup-pnpm/node_modules/.bin

ACCEPTANCE_SOURCE=${SEMLIA_ACCEPTANCE_SOURCE:-}
ACCEPTANCE_REF=${SEMLIA_ACCEPTANCE_REF:-}
if [ -z "${ACCEPTANCE_SOURCE}" ] || [ "${ACCEPTANCE_SOURCE#/}" = "${ACCEPTANCE_SOURCE}" ]; then
  printf '%s\n' 'SEMLIA_ACCEPTANCE_SOURCE must be an absolute local repository directory' >&2
  exit 2
fi
if [ ! -d "${ACCEPTANCE_SOURCE}" ]; then
  printf '%s\n' 'SEMLIA_ACCEPTANCE_SOURCE must be a local repository directory' >&2
  exit 2
fi
case "${ACCEPTANCE_REF}" in
  *[!0-9a-f]*|'')
    printf '%s\n' 'SEMLIA_ACCEPTANCE_REF must be an exact 40-character lowercase commit' >&2
    exit 2
    ;;
esac
if [ "${#ACCEPTANCE_REF}" -ne 40 ]; then
  printf '%s\n' 'SEMLIA_ACCEPTANCE_REF must be an exact 40-character lowercase commit' >&2
  exit 2
fi

LAUNCHER_ROOT=$(/usr/bin/mktemp -d /tmp/semlia-t008-launcher.XXXXXX) || {
  printf '%s\n' 'create isolated acceptance launcher directory: failed' >&2
  exit 1
}
cleanup_launcher() {
  /bin/chmod -R u+w "${LAUNCHER_ROOT}" 2>/dev/null || :
  /bin/rm -rf -- "${LAUNCHER_ROOT}"
}
trap cleanup_launcher 0
trap 'exit 130' HUP INT TERM

/bin/mkdir -p \
  "${LAUNCHER_ROOT}/home" \
  "${LAUNCHER_ROOT}/tmp" \
  "${LAUNCHER_ROOT}/docker-config" \
  "${LAUNCHER_ROOT}/config" \
  "${LAUNCHER_ROOT}/cache" \
  "${LAUNCHER_ROOT}/data" \
  "${LAUNCHER_ROOT}/state" \
  "${LAUNCHER_ROOT}/corepack" \
  "${LAUNCHER_ROOT}/go-build" \
  "${LAUNCHER_ROOT}/go-path" \
  "${LAUNCHER_ROOT}/go-mod" || exit 1
: >"${LAUNCHER_ROOT}/npmrc"

GO_BIN=
KERNEL_NAME=$(/usr/bin/uname -s 2>/dev/null || printf unknown)
MACHINE_NAME=$(/usr/bin/uname -m 2>/dev/null || printf unknown)
EXPECTED_GO_CANDIDATES=
case "${KERNEL_NAME}:${MACHINE_NAME}" in
  Linux:x86_64)
    EXPECTED_GO_CANDIDATES=/opt/hostedtoolcache/go/1.26.5/x64/bin/go:/usr/local/go/bin/go
    ;;
  Linux:aarch64|Linux:arm64)
    EXPECTED_GO_CANDIDATES=/opt/hostedtoolcache/go/1.26.5/arm64/bin/go:/usr/local/go/bin/go
    ;;
  Darwin:arm64)
    EXPECTED_GO_CANDIDATES=/Users/runner/hostedtoolcache/go/1.26.5/arm64/bin/go:/opt/homebrew/bin/go
    ;;
  Darwin:x86_64)
    EXPECTED_GO_CANDIDATES=/Users/runner/hostedtoolcache/go/1.26.5/x64/bin/go:/usr/local/go/bin/go
    ;;
esac
OLD_IFS=${IFS}
IFS=:
for CANDIDATE in ${EXPECTED_GO_CANDIDATES}; do
  if [ ! -x "${CANDIDATE}" ]; then
    continue
  fi
  GO_VERSION_OUTPUT=$(/usr/bin/env -i \
    "HOME=${LAUNCHER_ROOT}/home" \
    "PATH=${TRUSTED_PATH}" \
    GOENV=off \
    GOFLAGS= \
    GOWORK=off \
    GOTOOLCHAIN=local \
    "${CANDIDATE}" version 2>/dev/null) || continue
  case "${GO_VERSION_OUTPUT}" in
    "go version go${PINNED_GO_VERSION} "*|"go version go${PINNED_GO_BOOTSTRAP_VERSION} "*)
      GO_BIN=${CANDIDATE}
      break
      ;;
  esac
done
IFS=${OLD_IFS}
if [ -z "${GO_BIN}" ]; then
  printf 'fresh-clone acceptance requires Go %s at an explicit supported toolchain path\n' "${PINNED_GO_VERSION}" >&2
  exit 1
fi

/usr/bin/env -i \
  "HOME=${LAUNCHER_ROOT}/home" \
  "USER=${USER:-}" \
  "LOGNAME=${LOGNAME:-}" \
  "TMPDIR=${LAUNCHER_ROOT}/tmp" \
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
  "DOCKER_CONFIG=${LAUNCHER_ROOT}/docker-config" \
  "XDG_CONFIG_HOME=${LAUNCHER_ROOT}/config" \
  "XDG_CACHE_HOME=${LAUNCHER_ROOT}/cache" \
  "XDG_DATA_HOME=${LAUNCHER_ROOT}/data" \
  "XDG_STATE_HOME=${LAUNCHER_ROOT}/state" \
  "COREPACK_HOME=${LAUNCHER_ROOT}/corepack" \
  "npm_config_userconfig=${LAUNCHER_ROOT}/npmrc" \
  GIT_CONFIG_NOSYSTEM=1 \
  GOENV=off \
  GOFLAGS= \
  GOWORK=off \
  "GOTOOLCHAIN=go${PINNED_GO_VERSION}" \
  "GOCACHE=${LAUNCHER_ROOT}/go-build" \
  "GOPATH=${LAUNCHER_ROOT}/go-path" \
  "GOMODCACHE=${LAUNCHER_ROOT}/go-mod" \
  SEMLIA_RUN_FRESH_CLONE=1 \
  "SEMLIA_ACCEPTANCE_LAUNCHER_ROOT=${LAUNCHER_ROOT}" \
  "SEMLIA_ACCEPTANCE_SOURCE=${ACCEPTANCE_SOURCE}" \
  "SEMLIA_ACCEPTANCE_REF=${ACCEPTANCE_REF}" \
  "${GO_BIN}" -C "${ROOT}" test -v -timeout=100m -run '^TestFreshCloneAcceptance$' ./tests/acceptance/... -count=1
