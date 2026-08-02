#!/bin/sh

set -eu

[ "$#" -eq 1 ] || {
    printf 'usage: %s <rct-binary>\n' "$0" >&2
    exit 2
}

binary=$1
[ -f "$binary" ] || {
    printf 'test installer: binary not found: %s\n' "$binary" >&2
    exit 1
}

temporary_directory=$(mktemp -d "${TMPDIR:-/tmp}/rct-installer-test.XXXXXX")
trap 'rm -rf "$temporary_directory"' EXIT HUP INT TERM

case $(uname -s) in
    Darwin) os=darwin ;;
    Linux) os=linux ;;
    *) printf 'test installer: unsupported operating system\n' >&2; exit 1 ;;
esac

case $(uname -m) in
    arm64|aarch64) arch=arm64 ;;
    x86_64|amd64) arch=amd64 ;;
    *) printf 'test installer: unsupported architecture\n' >&2; exit 1 ;;
esac

version=vtest
asset="rct_${version}_${os}_${arch}.tar.gz"
release_directory="$temporary_directory/releases/$version"
stage_directory="$temporary_directory/stage"
install_directory="$temporary_directory/install"
mkdir -p "$release_directory" "$stage_directory"
cp "$binary" "$stage_directory/rct"
tar -czf "$release_directory/$asset" -C "$stage_directory" rct

if command -v shasum >/dev/null 2>&1; then
    checksum=$(shasum -a 256 "$release_directory/$asset" | awk '{ print $1 }')
else
    checksum=$(sha256sum "$release_directory/$asset" | awk '{ print $1 }')
fi
printf '%s  %s\n' "$checksum" "$asset" > "$release_directory/checksums.txt"

RCT_VERSION="$version" \
RCT_RELEASE_BASE_URL="file://$temporary_directory/releases" \
RCT_INSTALL_DIR="$install_directory" \
    sh ./scripts/install.sh

[ -x "$install_directory/rct" ] || {
    printf 'test installer: installed binary is not executable\n' >&2
    exit 1
}
"$install_directory/rct" version >/dev/null

RCT_INSTALL_DIR="$install_directory" sh ./scripts/uninstall.sh
[ ! -e "$install_directory/rct" ] || {
    printf 'test installer: uninstall left the binary behind\n' >&2
    exit 1
}

printf '%064d  %s\n' 0 "$asset" > "$release_directory/checksums.txt"
if RCT_VERSION="$version" \
    RCT_RELEASE_BASE_URL="file://$temporary_directory/releases" \
    RCT_INSTALL_DIR="$install_directory" \
    sh ./scripts/install.sh >/dev/null 2>&1; then
    printf 'test installer: checksum mismatch was accepted\n' >&2
    exit 1
fi
[ ! -e "$install_directory/rct" ] || {
    printf 'test installer: checksum failure installed a binary\n' >&2
    exit 1
}

printf 'Installer integration test passed.\n'
