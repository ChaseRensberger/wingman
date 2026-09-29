#!/usr/bin/env bash
set -euo pipefail

expected_version="${1:?usage: check-client-version.sh <version>}"
actual_version="$(node -p "require('./web/packages/client/package.json').version")"

if [[ "$actual_version" != "$expected_version" ]]; then
  printf 'client version %s does not match release version %s\n' "$actual_version" "$expected_version" >&2
  exit 1
fi

if ! grep -Fxq "    image: ghcr.io/chaserensberger/wingman:v$expected_version" compose.yaml; then
  printf 'Compose image does not match release version %s\n' "$expected_version" >&2
  exit 1
fi
