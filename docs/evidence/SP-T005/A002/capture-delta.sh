#!/usr/bin/env bash
set -euo pipefail
root="$(cd "$(dirname "$0")/../../../.." && pwd)"
cd "$root"
out=docs/evidence/SP-T005/A002
tmp="$(mktemp -d "${TMPDIR:-/tmp}/semlia-t005-a002-delta.XXXXXXXX")"
trap 'rm -rf "$tmp"' EXIT
tar -xf .semlia/evidence-work/SP-T005-A002-baseline.tar -C "$tmp"
{
  tar -tf .semlia/evidence-work/SP-T005-A002-baseline.tar
  rg --files internal/application/governance internal/domain/governance internal/adapters/postgres internal/platform/http migrations cmd/semlia tests/integration/governance tests/integration/db tests/contracts api sdk/typescript/src docs/specs/semantic-production db/sqlc.yaml
} | sort -u > "$tmp/paths"
: > "$out/source-delta.patch"
: > "$out/source-manifest.tsv"
while IFS= read -r path; do
  case "$path" in docs/evidence/*|*/) continue ;; esac
  before="$tmp/$path"
  after="$path"
  if ! test -f "$before" && ! test -f "$after"; then continue; fi
  if cmp -s "$before" "$after"; then continue; fi
  before_hash=absent
  after_hash=absent
  if test -f "$before"; then before_hash="$(shasum -a 256 "$before" | cut -d ' ' -f1)"; else before=/dev/null; fi
  if test -f "$after"; then after_hash="$(shasum -a 256 "$after" | cut -d ' ' -f1)"; else after=/dev/null; fi
  printf '%s\t%s\t%s\n' "$path" "$before_hash" "$after_hash" >> "$out/source-manifest.tsv"
  code=0
  diff -u --label "a/$path" --label "b/$path" "$before" "$after" >> "$out/source-delta.patch" || code=$?
  if test "$code" -gt 1; then exit "$code"; fi
done < "$tmp/paths"
