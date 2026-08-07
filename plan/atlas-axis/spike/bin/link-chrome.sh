#!/usr/bin/env bash
set -euo pipefail
# Per-job setup: make the shared Chrome visible inside this job's isolated HOME.
# Both browsing rungs run this, so the binary is identical across the ladder and
# cannot bias the comparison.
root="$(cat "$AXIS_CONFIG_DIR/.chrome-root")"
mkdir -p "$HOME/.sightmap"
[ -e "$HOME/.sightmap/browsers" ] || ln -s "$root" "$HOME/.sightmap/browsers"
