#!/bin/sh

set -eu

install_dir=${RCT_INSTALL_DIR:-"$HOME/.local/bin"}
target="$install_dir/rct"

if [ ! -e "$target" ]; then
    printf 'rct is not installed at %s\n' "$target"
    exit 0
fi

[ ! -d "$target" ] || {
    printf 'rct uninstall: refusing to remove directory %s\n' "$target" >&2
    exit 1
}

rm -f "$target"
printf 'Removed %s\n' "$target"
