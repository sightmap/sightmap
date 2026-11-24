#!/bin/bash
# Builds the Sightmap Sim Sample app for the iOS Simulator with xcodebuild and
# prints the .app path. Only Xcode is required.
#
# It is a real xcodebuild build on purpose: Xcode embeds the simulated
# entitlements a simulator app needs, which a hand-assembled bundle lacks.
#
# Env: OUT_DIR (default ${SIGHTMAP_DEVICE_CACHE:-~/Library/Caches/sightmap/device}/SampleApp).
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
cache_root="${SIGHTMAP_DEVICE_CACHE:-$HOME/Library/Caches/sightmap/device}"
out_dir="${OUT_DIR:-$cache_root/SampleApp}"
build_dir="$cache_root/SampleApp-build"
app="$out_dir/SimSample.app"

mkdir -p "$build_dir" "$out_dir"
echo "build.sh: xcodebuild for the iOS Simulator" >&2
xcodebuild -project "$here/SimSample.xcodeproj" -target SimSample -configuration Debug -sdk iphonesimulator \
  SYMROOT="$build_dir/products" OBJROOT="$build_dir/intermediates" INFOPLIST_FILE="$here/Info.plist" ARCHS="$(uname -m)" \
  build >"$build_dir/xcodebuild.log" 2>&1 \
  || { tail -20 "$build_dir/xcodebuild.log" >&2; echo "build.sh: xcodebuild failed; full log: $build_dir/xcodebuild.log" >&2; exit 1; }

rm -rf "$app"
cp -R "$build_dir/products/Debug-iphonesimulator/SimSample.app" "$app"
echo "$app"
