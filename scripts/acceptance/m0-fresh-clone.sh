#!/usr/bin/env -S SHELLOPTS= BASHOPTS= BASH_ENV= ENV= /bin/bash --noprofile --norc -p

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
umask 077
TRUSTED_PATH=/home/runner/setup-pnpm/node_modules/.bin:/Users/runner/setup-pnpm/node_modules/.bin
TRUSTED_PATH=${TRUSTED_PATH}:/opt/hostedtoolcache/node/24.15.0/x64/bin:/opt/hostedtoolcache/node/24.15.0/arm64/bin
TRUSTED_PATH=${TRUSTED_PATH}:/Users/runner/hostedtoolcache/node/24.15.0/x64/bin:/Users/runner/hostedtoolcache/node/24.15.0/arm64/bin
TRUSTED_PATH=${TRUSTED_PATH}:/opt/hostedtoolcache/go/1.26.5/x64/bin:/opt/hostedtoolcache/go/1.26.5/arm64/bin
TRUSTED_PATH=${TRUSTED_PATH}:/Users/runner/hostedtoolcache/go/1.26.5/x64/bin:/Users/runner/hostedtoolcache/go/1.26.5/arm64/bin
TRUSTED_PATH=${TRUSTED_PATH}:/usr/local/go/bin:/opt/homebrew/bin:/opt/homebrew/sbin:/home/linuxbrew/.linuxbrew/bin:/home/linuxbrew/.linuxbrew/sbin:/opt/local/bin:/opt/local/sbin:/usr/local/bin:/usr/local/sbin:/usr/bin:/bin:/usr/sbin:/sbin:/run/current-system/sw/bin:/nix/var/nix/profiles/default/bin:/snap/bin:/var/lib/snapd/snap/bin

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

LAUNCHER_ROOT=
CHILD_PID=
LAUNCHER_SIGNAL_STATUS=
LAUNCHER_SIGNAL_NAME=
cleanup_launcher() {
  if [ -z "${LAUNCHER_ROOT}" ]; then
    return 0
  fi
  /bin/chmod -R u+w "${LAUNCHER_ROOT}" 2>/dev/null || :
  /bin/rm -rf -- "${LAUNCHER_ROOT}" || return 1
  [ ! -e "${LAUNCHER_ROOT}" ]
}
cleanup_on_exit() {
  STATUS=$?
  trap - 0 HUP INT TERM
  if ! cleanup_launcher; then
    printf '%s\n' 'remove isolated acceptance launcher directory: failed' >&2
    if [ "${STATUS}" -eq 0 ]; then
      STATUS=1
    fi
  fi
  exit "${STATUS}"
}
forward_launcher_signal() {
  if [ -z "${LAUNCHER_SIGNAL_STATUS}" ]; then
    LAUNCHER_SIGNAL_STATUS=$1
    LAUNCHER_SIGNAL_NAME=$2
  fi
  if [ -z "${CHILD_PID}" ]; then
    return
  fi
  /bin/kill -"${LAUNCHER_SIGNAL_NAME}" -- "-${CHILD_PID}" 2>/dev/null || /bin/kill -"${LAUNCHER_SIGNAL_NAME}" "${CHILD_PID}" 2>/dev/null || :
}
trap cleanup_on_exit 0
trap 'forward_launcher_signal 129 HUP' HUP
trap 'forward_launcher_signal 130 INT' INT
trap 'forward_launcher_signal 143 TERM' TERM

LAUNCHER_ROOT=$(/usr/bin/mktemp -d /tmp/semlia-t008-launcher.XXXXXX) || {
  printf '%s\n' 'create isolated acceptance launcher directory: failed' >&2
  exit 1
}
/bin/chmod 700 "${LAUNCHER_ROOT}" || exit 1
wait_for_child() {
  while :; do
    wait "${CHILD_PID}"
    STATUS=$?
    if /bin/kill -0 "${CHILD_PID}" 2>/dev/null; then
      continue
    fi
    if [ -n "${LAUNCHER_SIGNAL_STATUS}" ]; then
      while /bin/kill -0 -- "-${CHILD_PID}" 2>/dev/null; do
        /bin/sleep 0.05
      done
      return "${LAUNCHER_SIGNAL_STATUS}"
    fi
    return "${STATUS}"
  done
}
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

NODE_BIN=
OLD_IFS=${IFS}
IFS=:
for DIRECTORY in ${TRUSTED_PATH}; do
  CANDIDATE=${DIRECTORY}/node
  if [ ! -x "${CANDIDATE}" ]; then
    continue
  fi
  NODE_VERSION_OUTPUT=$(/usr/bin/env -i "HOME=${HOME:-/var/empty}" "PATH=${TRUSTED_PATH}" NODE_USE_SYSTEM_CA=1 "${CANDIDATE}" --version 2>/dev/null) || continue
  if [ "${NODE_VERSION_OUTPUT}" = v24.15.0 ]; then
    NODE_BIN=${CANDIDATE}
    break
  fi
done
IFS=${OLD_IFS}
if [ -z "${NODE_BIN}" ]; then
  printf '%s\n' 'fresh-clone acceptance requires Node.js 24.15.0 at an explicit supported toolchain path' >&2
  exit 1
fi
SYSTEM_CA_BUNDLE=${LAUNCHER_ROOT}/system-ca.pem
SYSTEM_CA_DIAGNOSTIC=${LAUNCHER_ROOT}/system-ca.stderr
/usr/bin/env -i \
  "HOME=${HOME:-/var/empty}" \
  "PATH=${TRUSTED_PATH}" \
  NODE_USE_SYSTEM_CA=1 \
  "${NODE_BIN}" -e 'const {X509Certificate}=require("node:crypto");const tls=require("node:tls");const certificates=tls.getCACertificates("system");if(certificates.length===0)process.exit(42);for(const certificate of certificates){const lines=certificate.trim().split(/\r?\n/);if(lines[0]!=="-----BEGIN CERTIFICATE-----"||lines.at(-1)!=="-----END CERTIFICATE-----"||lines.length<3||lines.slice(1,-1).some(line=>!/^[A-Za-z0-9+/]+={0,2}$/.test(line)||line.length>64)){process.exit(43)}new X509Certificate(certificate)}process.stdout.write(certificates.map(certificate=>certificate.trim()).join("\n")+"\n")' \
  >"${SYSTEM_CA_BUNDLE}" 2>"${SYSTEM_CA_DIAGNOSTIC}" || {
    printf '%s\n' 'export a non-empty operating-system CA bundle for isolated Node.js: failed' >&2
    exit 1
  }
if [ -s "${SYSTEM_CA_DIAGNOSTIC}" ]; then
  printf '%s\n' 'export operating-system CA bundle produced unexpected diagnostics' >&2
  exit 1
fi
/bin/rm -f -- "${SYSTEM_CA_DIAGNOSTIC}" || exit 1
/bin/chmod 600 "${SYSTEM_CA_BUNDLE}" || exit 1
if [ ! -f "${SYSTEM_CA_BUNDLE}" ] || [ ! -s "${SYSTEM_CA_BUNDLE}" ] || [ -L "${SYSTEM_CA_BUNDLE}" ]; then
  printf '%s\n' 'isolated Node.js system CA bundle must be a non-empty physical file' >&2
  exit 1
fi

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

if [ -n "${LAUNCHER_SIGNAL_STATUS}" ]; then
  exit "${LAUNCHER_SIGNAL_STATUS}"
fi
set -m
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
  "PATH=${TRUSTED_PATH}" \
  "DOCKER_CONFIG=${LAUNCHER_ROOT}/docker-config" \
  "XDG_CONFIG_HOME=${LAUNCHER_ROOT}/config" \
  "XDG_CACHE_HOME=${LAUNCHER_ROOT}/cache" \
  "XDG_DATA_HOME=${LAUNCHER_ROOT}/data" \
  "XDG_STATE_HOME=${LAUNCHER_ROOT}/state" \
  "COREPACK_HOME=${LAUNCHER_ROOT}/corepack" \
  "NODE_EXTRA_CA_CERTS=${SYSTEM_CA_BUNDLE}" \
  NODE_USE_SYSTEM_CA=1 \
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
  "${GO_BIN}" -C "${ROOT}" test -v -timeout=100m -exec="/usr/bin/env PATH=${TRUSTED_PATH}" -run '^TestFreshCloneAcceptance$' ./tests/acceptance/... -count=1 &
CHILD_PID=$!
if ! /bin/kill -0 -- "-${CHILD_PID}" 2>/dev/null; then
  /bin/kill -TERM "${CHILD_PID}" 2>/dev/null || :
  wait "${CHILD_PID}" 2>/dev/null || :
  printf '%s\n' 'start fresh-clone acceptance in an isolated process group: failed' >&2
  exit 1
fi
if [ -n "${LAUNCHER_SIGNAL_STATUS}" ]; then
  /bin/kill -"${LAUNCHER_SIGNAL_NAME}" -- "-${CHILD_PID}" 2>/dev/null || /bin/kill -"${LAUNCHER_SIGNAL_NAME}" "${CHILD_PID}" 2>/dev/null || :
fi
set +m
wait_for_child
STATUS=$?
if [ -n "${LAUNCHER_SIGNAL_STATUS}" ]; then
  STATUS=${LAUNCHER_SIGNAL_STATUS}
fi
exit "${STATUS}"
