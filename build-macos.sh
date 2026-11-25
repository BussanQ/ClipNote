#!/bin/sh
set -eu
cd "$(dirname "$0")"
if [ "$(uname -s)" != Darwin ]; then
  echo "Use macOS and Go 1.27.3 or later to build and sign the app." >&2
  exit 1
fi
app="$PWD/dist/ClipNote.app"
mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources"
cp packaging/Info.plist "$app/Contents/Info.plist"
cp packaging/ClipNote.icns "$app/Contents/Resources/ClipNote.icns"
CGO_ENABLED=0 go build -buildvcs=false -trimpath -o "$app/Contents/MacOS/ClipNote" .
codesign --force --sign - "$app"
codesign --verify --strict "$app"
printf 'Built and verified signature: %s\n' "$app"
