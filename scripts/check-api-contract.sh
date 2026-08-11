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

while IFS= read -r line || [ -n "$line" ]; do
  case "$line" in
    output:*) printf 'output: %s\n' "$tmp/client.gen.go" ;;
    *) printf '%s\n' "$line" ;;
  esac
done <"$repo/client/oapi-codegen.yaml" >"$tmp/oapi-codegen.yaml"
go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0 \
  -config "$tmp/oapi-codegen.yaml" "$repo/openapi.json"
cmp "$repo/client/client.gen.go" "$tmp/client.gen.go"
