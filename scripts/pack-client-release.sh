#!/usr/bin/env bash
set -euo pipefail

usage() {
  printf 'usage: %s <version> <output-directory>\n' "${0##*/}" >&2
  exit 2
}

[[ $# -eq 2 ]] || usage
version=$1
[[ "$version" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] || usage

repo_root=$(cd "$(dirname "$0")/.." && pwd)
package_dir="$repo_root/web/packages/client"
mkdir -p "$2"
output_dir=$(cd "$2" && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

cp -R "$package_dir/dist" "$package_dir/LICENSE" "$package_dir/package.json" "$tmp/"
node - "$tmp/package.json" "$version" <<'NODE'
const fs = require("node:fs");
const [packagePath, version] = process.argv.slice(2);
const pkg = JSON.parse(fs.readFileSync(packagePath, "utf8"));
pkg.version = version;
fs.writeFileSync(packagePath, `${JSON.stringify(pkg, null, 2)}\n`);
NODE

(cd "$tmp" && npm pack --pack-destination "$output_dir" --json >/dev/null)
printf '%s/wingman-actor-client-%s.tgz\n' "$output_dir" "$version"
