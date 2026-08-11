#!/usr/bin/env bash
set -euo pipefail

readonly ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

cd "${ROOT}"

unformatted="$(find api cmd internal tests scripts -type f -name '*.go' -exec gofmt -l {} +)"
if [[ -n "${unformatted}" ]]; then
  printf 'Go files require gofmt:\n%s\n' "${unformatted}" >&2
  exit 1
fi

while IFS= read -r -d '' script; do
  bash -n "${script}"
done < <(find scripts -type f -name '*.sh' -print0)

printf 'Go formatting and shell syntax checks passed.\n'
