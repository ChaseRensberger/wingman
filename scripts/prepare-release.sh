#!/usr/bin/env bash
set -euo pipefail

usage() {
  printf 'usage: %s <version>\n' "${0##*/}" >&2
  printf 'example: %s 0.1.41\n' "${0##*/}" >&2
  exit 2
}

version="${1:-}"
if [[ ! "$version" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]]; then
  usage
fi

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
package_json="$repo_root/web/packages/client/package.json"
sdk_guide="$repo_root/web/apps/docs/src/content/docs/build-clients/typescript-sdk.md"

if git -C "$repo_root" rev-parse --verify --quiet "refs/tags/v$version" >/dev/null; then
  printf 'release tag v%s already exists locally\n' "$version" >&2
  exit 1
fi

if ! remote_tag="$(git -C "$repo_root" ls-remote --tags origin "refs/tags/v$version")"; then
  printf 'could not check origin for release tag v%s\n' "$version" >&2
  exit 1
fi
if [[ -n "$remote_tag" ]]; then
  printf 'release tag v%s already exists on origin\n' "$version" >&2
  exit 1
fi

node - "$version" "$package_json" "$sdk_guide" <<'NODE'
const fs = require("node:fs");

const [version, packagePath, guidePath] = process.argv.slice(2);
const packageJSON = JSON.parse(fs.readFileSync(packagePath, "utf8"));
const previousVersion = packageJSON.version;
const guide = fs.readFileSync(guidePath, "utf8");

if (!guide.includes(previousVersion)) {
  throw new Error(`${guidePath} does not mention package version ${previousVersion}`);
}

packageJSON.version = version;
fs.writeFileSync(packagePath, `${JSON.stringify(packageJSON, null, 2)}\n`);
fs.writeFileSync(guidePath, guide.replaceAll(previousVersion, version));
NODE

(cd "$repo_root/web" && bun install --lockfile-only)
(cd "$repo_root/web" && bun run generate:client)
"$repo_root/scripts/check-api-contract.sh"
(cd "$repo_root/web" && bun run --filter @wingman-actor/client test:package)

printf 'Prepared release v%s. Review the changes, commit them, then tag that commit.\n' "$version"
printf 'Suggested commit: build: prepare v%s release\n' "$version"
