#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "$0")/.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
fixture="$tmp/repo"
mkdir -p "$fixture/scripts" "$fixture/web/packages/client" "$tmp/extracted"
cp "$repo_root/scripts/pack-client-release.sh" "$fixture/scripts/"
cp -R "$repo_root/web/packages/client/dist" \
  "$repo_root/web/packages/client/LICENSE" \
  "$repo_root/web/packages/client/package.json" "$fixture/web/packages/client/"
cp -R "$fixture/web/packages/client" "$tmp/original"

version=99.0.0
tarball=$(bash "$fixture/scripts/pack-client-release.sh" "$version" "$tmp/release packages")
tar -xzf "$tarball" -C "$tmp/extracted"
node - "$tmp/extracted/package" "$version" <<'NODE'
const assert = require("node:assert/strict");
const fs = require("node:fs");
const [dir, version] = process.argv.slice(2);
const pkg = JSON.parse(fs.readFileSync(`${dir}/package.json`, "utf8"));
assert.equal(pkg.name, "@wingman-actor/client");
assert.equal(pkg.version, version);
for (const file of [pkg.main, pkg.types, "dist/schema.d.ts", "LICENSE"]) {
  assert.ok(fs.existsSync(`${dir}/${file}`), `missing package file: ${file}`);
}
NODE
bash "$repo_root/scripts/test-client-package.sh" "$tarball"
diff -r "$tmp/original" "$fixture/web/packages/client"

for invalid in '' v1.2.3 01.2.3 1.2 '1.2.3/../other'; do
  if bash "$fixture/scripts/pack-client-release.sh" "$invalid" "$tmp/invalid" >"$tmp/error" 2>&1; then
    printf 'accepted invalid release version: %s\n' "$invalid" >&2
    exit 1
  fi
done
[[ ! -e "$tmp/invalid" ]]
printf 'client release packaging test passed\n'
