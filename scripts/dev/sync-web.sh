#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
target="$root/internal/platform/web/static"

cd "$root"
pnpm --filter @semlia/web build

temporary="$(mktemp -d "$root/internal/platform/web/.static.XXXXXX")"
trap 'rm -rf "$temporary"' EXIT
cp -R "$root/web/dist/." "$temporary/"
rm -rf "$target"
mv "$temporary" "$target"
trap - EXIT
