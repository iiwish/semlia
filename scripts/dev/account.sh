#!/usr/bin/env bash
set -euo pipefail
set +x
cd "$(dirname "${BASH_SOURCE[0]}")/../.."

if [[ ! -t 0 ]]; then
  printf 'Run this command in your local terminal.\n' >&2
  exit 1
fi
if [[ "$#" -eq 4 && "$1" == "bootstrap-local-admin" ]] || [[ "$#" -eq 2 && "$1" == "reset-local-password" ]]; then
  IFS= read -r -s -p 'Password (at least 6 characters): ' password
  printf '\n'
  IFS= read -r -s -p 'Confirm password: ' confirmation
  printf '\n'
  trap 'unset password confirmation' EXIT
  if [[ "$password" != "$confirmation" ]]; then
    printf 'Passwords do not match.\n' >&2
    exit 1
  fi
  printf '%s\n' "$password" | node scripts/dev/native.mjs "$@"
else
  printf 'Usage: bash scripts/dev/account.sh bootstrap-local-admin <workspace-slug> <workspace-name> <username>\n       bash scripts/dev/account.sh reset-local-password <username>\n' >&2
  exit 2
fi
