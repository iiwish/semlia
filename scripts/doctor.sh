#!/usr/bin/env bash
set -euo pipefail

readonly REQUIRED_GO="1.26.6"
readonly REQUIRED_NODE="24.15.0"
readonly REQUIRED_PNPM="11.1.3"
readonly DEFAULT_PORTS="4173 8080 5433"

errors=0
warnings=0

ok() {
  printf '[ok] %s\n' "$1"
}

warn() {
  warnings=$((warnings + 1))
  printf '[warn] %s\n' "$1" >&2
}

error() {
  errors=$((errors + 1))
  printf '[error] %s\n' "$1" >&2
}

require_command() {
  local command_name="$1"
  local install_hint="$2"

  if command -v "${command_name}" >/dev/null 2>&1; then
    return 0
  fi

  error "${command_name} is missing. ${install_hint}"
  return 1
}

check_exact_version() {
  local label="$1"
  local actual="$2"
  local expected="$3"

  if [[ "${actual}" == "${expected}" ]]; then
    ok "${label} ${actual}"
  else
    error "${label} ${actual} is unsupported; expected ${expected}. Use .tool-versions."
  fi
}

printf 'Semlia environment doctor\n\n'

if require_command go "Install Go ${REQUIRED_GO}."; then
  go_version="$(go env GOVERSION)"
  check_exact_version "Go" "${go_version#go}" "${REQUIRED_GO}"

  go_module="$(go env GOMOD)"
  if [[ "${go_module}" == "/dev/null" ]]; then
    error "The current directory is not inside the Semlia Go module. Run make commands from the repository root."
  else
    ok "Go module detected"
  fi
fi

if require_command node "Install Node.js ${REQUIRED_NODE}."; then
  node_version="$(node --version)"
  check_exact_version "Node.js" "${node_version#v}" "${REQUIRED_NODE}"
fi

if require_command pnpm "Install pnpm ${REQUIRED_PNPM} with Corepack or the pnpm installer."; then
  check_exact_version "pnpm" "$(pnpm --version)" "${REQUIRED_PNPM}"
fi

if require_command git "Install a supported Git release."; then
  ok "$(git --version)"
fi

if require_command make "Install GNU Make."; then
  ok "$(make --version | head -n 1)"
fi

if require_command docker "Install Docker or a compatible Docker CLI and runtime."; then
  if docker info >/dev/null 2>&1; then
    ok "Docker daemon is reachable ($(docker info --format '{{.ServerVersion}}'))"
  else
    error "Docker CLI is installed, but the daemon is not reachable. Start the container runtime."
  fi
fi

if command -v lsof >/dev/null 2>&1; then
  for port in ${DEFAULT_PORTS}; do
    listener="$(lsof -nP -iTCP:"${port}" -sTCP:LISTEN 2>/dev/null | awk 'NR == 2 { print $1 " (pid " $2 ")" }' || true)"
    if [[ -n "${listener}" ]]; then
      warn "Default development port ${port} is in use by ${listener}. Override the service port before make dev."
    else
      ok "Default development port ${port} is available"
    fi
  done
else
  warn "lsof is unavailable; default development ports were not checked."
fi

printf '\nDoctor completed with %d warning(s) and %d error(s).\n' "${warnings}" "${errors}"

if [[ "${errors}" -ne 0 ]]; then
  exit 1
fi
