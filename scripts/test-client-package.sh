#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
package_dir="$repo_root/web/packages/client"
temp_dir="$(mktemp -d)"
trap 'rm -rf "$temp_dir"' EXIT

(cd "$package_dir" && npm pack --pack-destination "$temp_dir" --json >/dev/null)
tarball="$(printf '%s\n' "$temp_dir"/*.tgz)"
npm install --prefix "$temp_dir/consumer" "$tarball" --ignore-scripts --no-package-lock >/dev/null
node --input-type=module --eval "import('$temp_dir/consumer/node_modules/@wingman-actor/client/dist/index.js').then(({ createWingmanClient }) => { if (typeof createWingmanClient !== 'function') process.exit(1) })"
