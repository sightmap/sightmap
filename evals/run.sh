#!/usr/bin/env bash
# Build the sightmap CLI (unless SIGHTMAP_BIN is set) and run the eval cases.
# Usage: evals/run.sh [--only <case-id>] [--verbose]
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
bin=${SIGHTMAP_BIN:-}

if [ -z "$bin" ]; then
  bin=$(mktemp -d)/sightmap
  echo "building sightmap → $bin"
  (cd "$root/go" && go build -o "$bin" ./cmd/sightmap)
fi

exec go -C "$root/go" run ./cmd/sightmap-evals \
  --bin "$bin" \
  --cases "$root/evals/cases" \
  --repo "$root" \
  "$@"
