#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "$0")/.." && pwd)
previous_version=$(node -p "require('$repo_root/web/packages/client/package.json').version")
next_version=99.0.0
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
fixture=$tmp/repo
mkdir -p "$fixture/scripts" "$fixture/web/packages/client" \
  "$fixture/web/apps/docs/src/content/docs/build-clients" "$tmp/bin"
cp "$repo_root/scripts/prepare-release.sh" "$repo_root/scripts/check-client-version.sh" "$fixture/scripts/"
cp "$repo_root/compose.yaml" "$fixture/compose.yaml"
printf '{"version":"%s"}\n' "$previous_version" > "$fixture/web/packages/client/package.json"
printf 'Install @wingman-actor/client@%s\n' "$previous_version" > "$fixture/web/apps/docs/src/content/docs/build-clients/typescript-sdk.md"
printf '#!/bin/sh\nexit 0\n' > "$fixture/scripts/check-api-contract.sh"
printf '#!/bin/sh\nexit 0\n' > "$tmp/bin/bun"
cat > "$tmp/bin/git" <<'SH'
#!/bin/sh
if [ "$3" = rev-parse ]; then exit 1; fi
exit 0
SH
chmod +x "$fixture/scripts/check-api-contract.sh" "$tmp/bin/bun" "$tmp/bin/git"

PATH="$tmp/bin:$PATH" bash "$fixture/scripts/prepare-release.sh" "$next_version" >/dev/null
(cd "$fixture" && bash scripts/check-client-version.sh "$next_version")
grep -Fxq "    image: ghcr.io/chaserensberger/wingman:v$next_version" "$fixture/compose.yaml"
grep -Fq "@wingman-actor/client@$next_version" "$fixture/web/apps/docs/src/content/docs/build-clients/typescript-sdk.md"

sed -i "s/wingman:v$next_version/wingman:v$previous_version/" "$fixture/compose.yaml"
if PATH="$tmp/bin:$PATH" bash "$fixture/scripts/prepare-release.sh" 99.0.1 >/dev/null 2>&1; then
  printf 'release preparation accepted an out-of-date Compose image\n' >&2
  exit 1
fi
grep -Fq "\"version\": \"$next_version\"" "$fixture/web/packages/client/package.json"
printf 'release preparation test passed\n'
