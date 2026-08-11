#!/usr/bin/env bash
set -euo pipefail

repo=$(cd "$(dirname "$0")/.." && pwd)
tmp=$(mktemp -d)

cleanup() {
  rm -rf "$tmp"
}
trap cleanup EXIT

fail() {
  printf 'installer test failed: %s\n' "$*" >&2
  exit 1
}

binary=$tmp/wingman
printf '#!/usr/bin/env bash\nexit 0\n' > "$binary"
chmod +x "$binary"

archive=$tmp/wingman_1.2.3_linux_amd64.tar.gz
archive_dir=$tmp/archive
checksums=$tmp/checksums.txt
mkdir -p "$archive_dir"
cp "$binary" "$archive_dir/wingman"
tar -czf "$archive" -C "$archive_dir" wingman
sha256sum "$archive" | awk '{print $1 "  wingman_1.2.3_linux_amd64.tar.gz"}' > "$checksums"

mock_bin=$tmp/mock-bin
mkdir -p "$mock_bin"
printf '%s\n' \
  '#!/usr/bin/env bash' \
  'set -euo pipefail' \
  '' \
  'url=""' \
  'output=""' \
  'while [[ $# -gt 0 ]]; do' \
  '  case "$1" in' \
  '    -o)' \
  '      output=$2' \
  '      shift 2' \
  '      ;;' \
  '    http*)' \
  '      url=$1' \
  '      shift' \
  '      ;;' \
  '    *) shift ;;' \
  '  esac' \
  'done' \
  '' \
  'if [[ "$url" == *"api.github.com"* ]]; then' \
  '  printf "{\\"browser_download_url\\":\\"https://example.test/wingman_1.2.3_linux_amd64.tar.gz\\"}\\n"' \
  'elif [[ "$url" == *"checksums.txt" ]]; then' \
  '  cp "$WINGMAN_TEST_CHECKSUMS" "$output"' \
  'else' \
  '  cp "$WINGMAN_TEST_ARCHIVE" "$output"' \
  'fi' \
  > "$mock_bin/curl"
chmod +x "$mock_bin/curl"

run_install() {
  local shell=$1 home=$2 install_dir=$3
  HOME=$home SHELL=$shell "$repo/install" --binary "$binary" --install-dir "$install_dir" --yes
}

location_home=$tmp/location-home
location_output=$tmp/location-output
HOME=$location_home XDG_CONFIG_HOME=$location_home/.config XDG_STATE_HOME=$location_home/.local/state "$repo/install" --binary "$binary" --install-dir "$location_home/bin" --no-modify-path --yes >"$location_output"
grep -Fxq "  Binary (created now): $location_home/bin/wingman" "$location_output" || fail 'binary location was not reported'
grep -Fxq "  Config (created when saved): $location_home/.config/wingman/wingman.json" "$location_output" || fail 'config location was not reported'
grep -Fxq "  Database (created when the server starts): $location_home/.local/share/wingman/wingman.db" "$location_output" || fail 'database location was not reported'
grep -Fxq "  Daemon state (created by serve): $location_home/.local/state/wingman" "$location_output" || fail 'daemon state location was not reported'
grep -Fxq "Installed wingman to $location_home/bin" "$location_output" || fail 'plain install success was not reported'
grep -Fxq 'Verify: wingman version' "$location_output" || fail 'verify next step was not reported'
grep -Fxq 'Start: wingman service start' "$location_output" || fail 'start next step was not reported'
grep -Fxq 'Docs: https://docs.wingman.actor' "$location_output" || fail 'docs next step was not reported'

archive_home=$tmp/archive-home
PATH="$mock_bin:$PATH" WINGMAN_TEST_ARCHIVE=$archive WINGMAN_TEST_CHECKSUMS=$checksums HOME=$archive_home "$repo/install" --version 1.2.3 --install-dir "$archive_home/bin" --no-modify-path --yes >"$tmp/archive-output"
[[ -x $archive_home/bin/wingman ]] || fail 'archive install did not produce wingman'
grep -Fq 'Installing wingman v1.2.3 (linux/amd64)' "$tmp/archive-output" || fail 'archive install did not use the detected platform'

bash_home=$tmp/bash-home
mkdir -p "$bash_home"
touch "$bash_home/.bashrc"
touch "$bash_home/.bash_profile"
run_install /bin/bash "$bash_home" "$bash_home/bin"
grep -Fxq "export PATH=\"$bash_home/bin:\$PATH\"" "$bash_home/.bashrc" || fail 'bash PATH entry was not added'
grep -Fxq "export PATH=\"$bash_home/bin:\$PATH\"" "$bash_home/.bash_profile" || fail 'bash login PATH entry was not added'
[[ ! -e $bash_home/.zshrc ]] || fail 'bash install created .zshrc'

fish_home=$tmp/fish-home
mkdir -p "$fish_home/.config/fish"
touch "$fish_home/.config/fish/config.fish"
fish_install_dir="$fish_home/bin with spaces"
run_install /usr/bin/fish "$fish_home" "$fish_install_dir"
grep -Fxq "fish_add_path \"$fish_install_dir\"" "$fish_home/.config/fish/config.fish" || fail 'fish PATH entry was not added'

github_path=$tmp/github-path
github_home=$tmp/github-home
mkdir -p "$github_home"
GITHUB_ACTIONS=true GITHUB_PATH=$github_path HOME=$github_home SHELL=/bin/bash "$repo/install" --binary "$binary" --install-dir "$github_home/bin" --yes
grep -Fxq "$github_home/bin" "$github_path" || fail 'GitHub Actions PATH entry was not added'
[[ ! -e $github_home/.bashrc ]] || fail 'GitHub Actions install modified .bashrc'

skip_path_home=$tmp/skip-path-home
mkdir -p "$skip_path_home"
touch "$skip_path_home/.bashrc"
HOME=$skip_path_home SHELL=/bin/bash "$repo/install" --binary "$binary" --install-dir "$skip_path_home/bin" --no-modify-path --yes
if grep -Fq "$skip_path_home/bin" "$skip_path_home/.bashrc"; then
  fail '--no-modify-path install modified PATH'
fi

windows_home=$tmp/windows-home
windows_binary=$tmp/wingman.exe
cp "$binary" "$windows_binary"
printf '#!/usr/bin/env bash\nif [[ "$1" == "-s" ]]; then\n  printf "MINGW64_NT-10.0\\n"\nelse\n  printf "x86_64\\n"\nfi\n' > "$mock_bin/uname"
chmod +x "$mock_bin/uname"
PATH="$mock_bin:$PATH" HOME=$windows_home "$repo/install" --binary "$windows_binary" --install-dir "$windows_home/bin" --no-modify-path --yes >"$tmp/windows-output"
[[ -x $windows_home/bin/wingman.exe ]] || fail 'Windows install did not produce wingman.exe'
grep -Fq 'Managed service: not supported; use wingman serve' "$tmp/windows-output" || fail 'Windows service limitation was not reported'

unsupported_home=$tmp/unsupported-home
printf '#!/usr/bin/env bash\nprintf "FreeBSD\\n"\n' > "$mock_bin/uname"
chmod +x "$mock_bin/uname"
if PATH="$mock_bin:$PATH" HOME=$unsupported_home "$repo/install" --binary "$binary" --install-dir "$unsupported_home/bin" --yes >"$tmp/unsupported-output" 2>&1; then
  fail 'unsupported operating system succeeded'
fi
grep -Fq 'Unsupported operating system' "$tmp/unsupported-output" || fail 'unsupported operating system error was not reported'
[[ ! -e $unsupported_home ]] || fail 'unsupported operating system created an install directory'

printf 'installer tests passed\n'
