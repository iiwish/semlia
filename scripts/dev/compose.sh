#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
"$root/scripts/dev/ensure-env.sh"
cd "$root"
exec docker compose --env-file "$root/.semlia/dev.env" "$@"
