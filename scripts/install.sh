#!/bin/sh

set -eu

fail() {
    printf 'rct install: %s\n' "$*" >&2
    exit 1
}

require_command() {
    command -v "$1" >/dev/null 2>&1 || fail "required command not found: $1"
}

repository=${RCT_REPOSITORY:-hironeko/rct}
version=${RCT_VERSION:-latest}
install_dir=${RCT_INSTALL_DIR:-"$HOME/.local/bin"}
release_base_url=${RCT_RELEASE_BASE_URL:-"https://github.com/${repository}/releases/download"}

for command in curl tar awk install mktemp uname; do
    require_command "$command"
done

case $(uname -s) in
    Darwin) os=darwin ;;
    Linux) os=linux ;;
    *) fail "unsupported operating system: $(uname -s)" ;;
esac

case $(uname -m) in
    arm64|aarch64) arch=arm64 ;;
    x86_64|amd64) arch=amd64 ;;
    *) fail "unsupported architecture: $(uname -m)" ;;
esac

if [ "$version" = latest ]; then
    [ -z "${RCT_RELEASE_BASE_URL:-}" ] || fail "RCT_VERSION is required with RCT_RELEASE_BASE_URL"
    latest_url=$(curl -fsSL -o /dev/null -w '%{url_effective}' "https://github.com/${repository}/releases/latest")
    version=${latest_url##*/}
    [ -n "$version" ] && [ "$version" != latest ] || fail "could not resolve the latest release"
fi

asset="rct_${version}_${os}_${arch}.tar.gz"
temporary_directory=$(mktemp -d "${TMPDIR:-/tmp}/rct-install.XXXXXX")
trap 'rm -rf "$temporary_directory"' EXIT HUP INT TERM

archive="$temporary_directory/$asset"
checksums="$temporary_directory/checksums.txt"
release_url="${release_base_url}/${version}"

curl -fsSL "${release_url}/${asset}" -o "$archive"
curl -fsSL "${release_url}/checksums.txt" -o "$checksums"

expected=$(awk -v asset="$asset" '$2 == asset { print $1; exit }' "$checksums")
[ -n "$expected" ] || fail "checksum is missing for $asset"

if command -v shasum >/dev/null 2>&1; then
    actual=$(shasum -a 256 "$archive" | awk '{ print $1 }')
elif command -v sha256sum >/dev/null 2>&1; then
    actual=$(sha256sum "$archive" | awk '{ print $1 }')
else
    fail "shasum or sha256sum is required"
fi

[ "$actual" = "$expected" ] || fail "checksum verification failed for $asset"

contents=$(tar -tzf "$archive")
[ "$contents" = rct ] || fail "release archive contains unexpected paths"

extract_directory="$temporary_directory/extract"
mkdir -p "$extract_directory"
tar -xzf "$archive" -C "$extract_directory"
[ -f "$extract_directory/rct" ] || fail "release archive does not contain rct"

install -d "$install_dir"
install -m 0755 "$extract_directory/rct" "$install_dir/rct"

printf 'Installed rct %s to %s\n' "$version" "$install_dir/rct"
case ":${PATH}:" in
    *":${install_dir}:"*) ;;
    *) printf 'Add %s to PATH before invoking rct.\n' "$install_dir" ;;
esac
