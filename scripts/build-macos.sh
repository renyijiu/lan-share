#!/bin/sh
set -eu
project_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$project_dir"
output_dir=${1:-"$project_dir/dist"}
mkdir -p "$output_dir"
output_dir=$(CDPATH= cd -- "$output_dir" && pwd)
app_dir="$output_dir/LAN Share.app"
build_dir=$(mktemp -d "${TMPDIR:-/tmp}/lan-share-build.XXXXXX")
trap 'rm -rf "$build_dir"' EXIT HUP INT TERM
mkdir -p "$app_dir/Contents/MacOS" "$app_dir/Contents/Resources"
for arch in arm64 amd64; do
  CGO_ENABLED=0 GOOS=darwin GOARCH="$arch" go build -trimpath -tags mygo_noinspector \
    -ldflags '-s -w -X github.com/egoist/mygo.production=1 -X github.com/egoist/mygo.packageIdentifier=dev.lanshare.desktop -X github.com/egoist/mygo.packageVersion=0.1.0' \
    -o "$build_dir/lan-share-$arch" ./cmd/lan-share
  CGO_ENABLED=0 GOOS=darwin GOARCH="$arch" go build -trimpath -ldflags '-s -w' \
    -o "$build_dir/helper-$arch" ./cmd/port-helper
done
lipo -create "$build_dir/lan-share-arm64" "$build_dir/lan-share-amd64" -output "$app_dir/Contents/MacOS/lan-share"
lipo -create "$build_dir/helper-arm64" "$build_dir/helper-amd64" -output "$app_dir/Contents/MacOS/lan-share-port-helper"
cp resources/Info.plist "$app_dir/Contents/Info.plist"
cp -R licenses "$app_dir/Contents/Resources/"
cp THIRD_PARTY_NOTICES.md "$app_dir/Contents/Resources/"
if [ -f resources/AppIcon.icns ]; then cp resources/AppIcon.icns "$app_dir/Contents/Resources/AppIcon.icns"; fi
codesign --force --sign - "$app_dir/Contents/MacOS/lan-share-port-helper"
codesign --force --sign - "$app_dir"
codesign --verify --deep --strict "$app_dir"
ditto -c -k --sequesterRsrc --keepParent "$app_dir" "$output_dir/LAN-Share-macOS.zip"
echo "Built: $app_dir"
