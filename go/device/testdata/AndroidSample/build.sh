#!/bin/bash
# Builds the Sightmap Android sample (testdata/AndroidSample) and prints the
# .apk path. Needs JDK 17 (JAVA_HOME, or java on PATH) and the Android SDK
# (ANDROID_HOME, else ~/Library/Android/sdk or ~/Android/Sdk); Gradle
# downloads the SDK platform and libraries it lacks on the first build.
#
# Gradle is $GRADLE when set, else the pinned distribution below, downloaded
# once into the cache and checked against its published SHA-256. There is no
# Gradle wrapper on purpose: its jar would be the repository's only binary.
#
# Env: GRADLE, OUT_DIR (default <cache>/AndroidSample), SIGHTMAP_DEVICE_CACHE
# (default ~/Library/Caches/sightmap/device on macOS, else
# ${XDG_CACHE_HOME:-~/.cache}/sightmap/device).
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
gradle_version=9.0.0
gradle_sha256=8fad3d78296ca518113f3d29016617c7f9367dc005f932bd9d93bf45ba46072b

if [ "$(uname)" = Darwin ]; then
  default_cache="$HOME/Library/Caches/sightmap/device"
else
  default_cache="${XDG_CACHE_HOME:-$HOME/.cache}/sightmap/device"
fi
cache_root="${SIGHTMAP_DEVICE_CACHE:-$default_cache}"
out_dir="${OUT_DIR:-$cache_root/AndroidSample}"

if [ -z "${ANDROID_HOME:-}" ]; then
  for sdk in "$HOME/Library/Android/sdk" "$HOME/Android/Sdk"; do
    [ -d "$sdk" ] && export ANDROID_HOME="$sdk" && break
  done
fi
[ -n "${ANDROID_HOME:-}" ] || { echo "build.sh: no Android SDK; set ANDROID_HOME" >&2; exit 1; }

gradle="${GRADLE:-}"
if [ -z "$gradle" ]; then
  dist="$cache_root/gradle-$gradle_version"
  gradle="$dist/bin/gradle"
  if [ ! -x "$gradle" ]; then
    echo "build.sh: downloading Gradle $gradle_version" >&2
    mkdir -p "$cache_root"
    zip="$cache_root/gradle-$gradle_version-bin.zip"
    curl -fsSL -o "$zip" "https://services.gradle.org/distributions/gradle-$gradle_version-bin.zip"
    sum="$( (shasum -a 256 "$zip" 2>/dev/null || sha256sum "$zip") | cut -d' ' -f1)"
    if [ "$sum" != "$gradle_sha256" ]; then
      rm -f "$zip"
      echo "build.sh: Gradle download has SHA-256 $sum, want $gradle_sha256" >&2
      exit 1
    fi
    rm -rf "$dist"
    unzip -q "$zip" -d "$cache_root"
    rm -f "$zip"
  fi
fi

log="$cache_root/AndroidSample-gradle.log"
mkdir -p "$out_dir"
echo "build.sh: gradle assembleDebug" >&2
(cd "$here" && "$gradle" --no-daemon --console=plain assembleDebug) >"$log" 2>&1 \
  || { tail -20 "$log" >&2; echo "build.sh: gradle failed; full log: $log" >&2; exit 1; }

apk="$out_dir/AndroidSample.apk"
cp "$here/app/build/outputs/apk/debug/app-debug.apk" "$apk"
echo "$apk"
