#!/usr/bin/env bash
set -euo pipefail

image=${1:-wingman:container-test}
name="wingman-smoke-$$"
volume="wingman-smoke-$$"
mkdir -p "${TMPDIR:-/tmp/opencode}"
tmp=$(mktemp -d "${TMPDIR:-/tmp/opencode}/wingman-container.XXXXXX")
chmod 755 "$tmp"

cleanup() {
  docker rm -f "$name" >/dev/null 2>&1 || true
  docker volume rm "$volume" >/dev/null 2>&1 || true
  rm -rf "$tmp"
}
trap cleanup EXIT

docker volume create "$volume" >/dev/null
docker run -d --name "$name" -p 127.0.0.1::2424 \
  -v "$volume:/data" -v "$tmp:/workspace:ro" \
  -e WINGMAN_USERNAME=smoke -e WINGMAN_PASSWORD=smoke-password \
  "$image" >/dev/null
port=$(docker port "$name" 2424/tcp)
url="http://$port"

ready() {
  local status
  for _ in {1..100}; do
    status=$(curl -sS -o /dev/null -w '%{http_code}' --max-time 2 "$url/health" 2>/dev/null) || status=000
    [[ $status == 200 ]] && return
    sleep 0.1
  done
  docker logs "$name" >&2
  return 1
}

ready
[[ $(curl -sS -o /dev/null -w '%{http_code}' "$url/ready") == 401 ]]
[[ $(curl -sS -o /dev/null -w '%{http_code}' -u smoke:smoke-password "$url/ready") == 200 ]]
[[ $(curl -sS -o "$tmp/console.html" -w '%{http_code}' -u smoke:smoke-password "$url/console/") == 200 ]]
grep -q '/console/assets/.*\.js' "$tmp/console.html"
console=$(<"$tmp/console.html")
[[ $console =~ src=\"/console/assets/([^\"]+\.js)\" ]]
[[ $(curl -sS -o /dev/null -w '%{http_code}' -u smoke:smoke-password \
  "$url/console/assets/${BASH_REMATCH[1]}") == 200 ]]

printf 'container mount\n' > "$tmp/test.txt"
docker exec "$name" test -f /workspace/test.txt
[[ $(curl -sS -o /dev/null -w '%{http_code}' -u smoke:smoke-password \
  -H 'Content-Type: application/json' -d '{"id":"cli_smoke","name":"Smoke"}' "$url/clients") == 201 ]]

docker stop "$name" >/dev/null
docker start "$name" >/dev/null
port=$(docker port "$name" 2424/tcp)
url="http://$port"
ready
curl -fsS -u smoke:smoke-password "$url/clients" | grep -q 'cli_smoke'
printf 'container smoke test passed\n'
