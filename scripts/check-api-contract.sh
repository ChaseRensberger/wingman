#!/bin/sh
set -eu

repo=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

cd "$repo"
go run ./cmd/openapi -output "$tmp/openapi.json"
cmp "$repo/openapi.json" "$tmp/openapi.json"

"$repo/web/packages/client/node_modules/.bin/openapi-typescript" \
  "$repo/openapi.json" -o "$tmp/schema.d.ts" >/dev/null
cmp "$repo/web/packages/client/src/schema.d.ts" "$tmp/schema.d.ts"
