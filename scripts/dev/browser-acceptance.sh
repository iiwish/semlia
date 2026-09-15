#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/../.."
node scripts/dev/validate-native.mjs
bash scripts/dev/production-acceptance.sh --suite desktop
