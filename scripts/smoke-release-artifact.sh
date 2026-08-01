#!/usr/bin/env bash
set -euo pipefail

usage() {
  printf 'usage: %s RELEASE_ARCHIVE\n' "${0##*/}" >&2
  exit 2
}

fail() {
  printf 'smoke test failed: %s\n' "$*" >&2
  exit 1
}

[[ $# -eq 1 ]] || usage
archive=$1
[[ -f $archive ]] || fail "release archive does not exist: $archive"

case $archive in
  *.tar.gz|*.zip) ;;
  *) fail "release archive must end in .tar.gz or .zip" ;;
esac

for command in curl find mktemp; do
  command -v "$command" >/dev/null 2>&1 || fail "required command not found: $command"
done

case $archive in
  *.tar.gz) command -v tar >/dev/null 2>&1 || fail 'required command not found: tar' ;;
  *.zip) command -v unzip >/dev/null 2>&1 || fail 'required command not found: unzip' ;;
esac

tmp=$(mktemp -d)
daemon_pid=
port=${WINGMAN_SMOKE_PORT:-$((20000 + RANDOM % 20000))}

cleanup() {
  status=$?
  trap - EXIT
  if [[ -n $daemon_pid ]]; then
    kill -TERM "$daemon_pid" 2>/dev/null || true
    for _ in {1..50}; do
      kill -0 "$daemon_pid" 2>/dev/null || break
      sleep 0.1
    done
    kill -KILL "$daemon_pid" 2>/dev/null || true
    wait "$daemon_pid" 2>/dev/null || true
  fi
  rm -rf "$tmp"
  exit "$status"
}

trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM HUP

extract_dir=$tmp/extract
state_dir=$tmp/state
mkdir -p "$extract_dir" "$state_dir" "$tmp/home" "$tmp/config" "$tmp/data"

case $archive in
  *.tar.gz) tar -xzf "$archive" -C "$extract_dir" ;;
  *.zip) unzip -q "$archive" -d "$extract_dir" ;;
esac

binary=$(find "$extract_dir" -type f \( -name wingman -o -name wingman.exe \) -print -quit)
[[ -n $binary ]] || fail 'wingman binary not found in release archive'
chmod u+x "$binary"

version_output=$("$binary" version)
[[ -n $version_output ]] || fail 'wingman version returned no output'

HOME=$tmp/home
XDG_CONFIG_HOME=$tmp/config
XDG_DATA_HOME=$tmp/data
XDG_STATE_HOME=$tmp/data/state
export HOME XDG_CONFIG_HOME XDG_DATA_HOME XDG_STATE_HOME

"$binary" serve --ephemeral --no-plugins --register --state-dir "$state_dir" --host 127.0.0.1 --port "$port" \
  >"$tmp/daemon.log" 2>&1 &
daemon_pid=$!

registration_file=$state_dir/registration.json
credential_file=$state_dir/credential
for _ in {1..100}; do
  [[ -f $registration_file && -f $credential_file ]] && break
  kill -0 "$daemon_pid" 2>/dev/null || fail 'daemon exited before publishing registration'
  sleep 0.1
done
[[ -f $registration_file ]] || fail 'daemon did not publish registration'
[[ -f $credential_file ]] || fail 'daemon did not publish credential'

registration=$(<"$registration_file")
credential=$(<"$credential_file")
[[ $registration =~ \"instance_id\":\"[^\"]+\" ]] || fail 'registration has no instance_id'
if [[ $registration =~ \"url\":\"([^\"]+)\" ]]; then
  daemon_url=${BASH_REMATCH[1]}
else
  fail 'registration has no URL'
fi
[[ $credential =~ ^[A-Za-z0-9_-]+$ ]] || fail 'credential is invalid'

for _ in {1..100}; do
  health_status=$(curl --silent --show-error --output "$tmp/health" --write-out '%{http_code}' \
    --connect-timeout 1 --max-time 2 "$daemon_url/health") || health_status=000
  [[ $health_status == 200 ]] && break
  kill -0 "$daemon_pid" 2>/dev/null || fail 'daemon exited before becoming healthy'
  sleep 0.1
done
[[ $health_status == 200 ]] || fail "public /health returned $health_status"

ready_status=$(curl --silent --show-error --output "$tmp/ready-unauthenticated" --write-out '%{http_code}' \
  --connect-timeout 1 --max-time 2 "$daemon_url/ready") || ready_status=000
[[ $ready_status == 401 ]] || fail "unauthenticated /ready returned $ready_status"

ready_status=$(curl --silent --show-error --output "$tmp/ready-authenticated" --write-out '%{http_code}' \
  --connect-timeout 1 --max-time 2 --header "Authorization: Bearer $credential" "$daemon_url/ready") || ready_status=000
[[ $ready_status == 200 ]] || fail "authenticated /ready returned $ready_status"

console_status=$(curl --silent --show-error --output "$tmp/console" --write-out '%{http_code}' \
  --connect-timeout 1 --max-time 2 "$daemon_url/console/") || console_status=000
[[ $console_status == 200 && -s $tmp/console ]] || fail "/console/ returned $console_status without content"

kill -TERM "$daemon_pid"
wait "$daemon_pid"
daemon_pid=

[[ ! -e $registration_file ]] || fail 'daemon registration was not removed on shutdown'
printf 'release artifact smoke test passed\n'
