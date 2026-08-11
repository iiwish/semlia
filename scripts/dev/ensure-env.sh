#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
state_dir="$root/.semlia"
env_file="$state_dir/dev.env"

random_hex() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex 32
    return
  fi
  od -An -N32 -tx1 /dev/urandom | tr -d ' \n'
}

umask 077
mkdir -p "$state_dir"

if [[ -L "$env_file" ]]; then
  printf 'local environment file must not be a symbolic link\n' >&2
  exit 1
fi

if [[ -f "$env_file" ]]; then
  chmod 600 "$env_file"
  if ! grep -Eq '^SEMLIA_POSTGRES_PASSWORD=[0-9a-f]{64}$' "$env_file" ||
    ! grep -Eq '^SEMLIA_SECRET_KEY=[0-9a-f]{64}$' "$env_file"; then
    printf 'local environment file is invalid; remove .semlia/dev.env to regenerate it\n' >&2
    exit 1
  fi
  exit 0
fi

build_version="local"
if command -v git >/dev/null 2>&1; then
  revision="$(git -C "$root" rev-parse --short HEAD 2>/dev/null || true)"
  if [[ -n "$revision" ]]; then
    build_version="local-$revision"
  fi
fi

temporary="$(mktemp "$state_dir/dev.env.XXXXXX")"
trap 'rm -f "$temporary"' EXIT
{
  printf 'COMPOSE_PROJECT_NAME=semlia-local\n'
  printf 'SEMLIA_HTTP_PORT=8080\n'
  printf 'SEMLIA_POSTGRES_PORT=5433\n'
  printf 'SEMLIA_BUILD_VERSION=%s\n' "$build_version"
  printf 'SEMLIA_POSTGRES_PASSWORD=%s\n' "$(random_hex)"
  printf 'SEMLIA_SECRET_KEY=%s\n' "$(random_hex)"
} >"$temporary"
chmod 600 "$temporary"
mv "$temporary" "$env_file"
trap - EXIT
