#!/bin/sh
# Builds the download for a GitHub release:
#   dist/super_v_clipboard-linux-amd64.tar.gz
# Upload that file to a new release on GitHub. Keep the file name the same,
# so the README's download link always gets the latest version.
set -e
cd "$(dirname "$0")"

rm -rf dist
mkdir -p dist/super_v_clipboard
go build -trimpath -ldflags="-s -w" -o dist/super_v_clipboard/super_v_clipboard .
cp README.md LICENSE super_v_clipboard.service 60-super-v-clipboard-uinput.rules dist/super_v_clipboard/
tar czf dist/super_v_clipboard-linux-amd64.tar.gz -C dist super_v_clipboard

echo "Done: dist/super_v_clipboard-linux-amd64.tar.gz"
