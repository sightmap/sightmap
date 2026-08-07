#!/usr/bin/env bash
set -euo pipefail
# beforeAll: install the managed Chrome for Testing once on the host and record
# where it lives, so per-job workspaces (isolated HOME) can symlink to it.
# Runs with the config directory as cwd.
command -v sightmap >/dev/null || {
  echo "sightmap CLI not on PATH — npm install -g @sightmap/sightmap" >&2
  exit 1
}
# AXIS scrubs HOME from the beforeAll environment (its passthrough list is
# PATH/USER/SHELL/LANG/TERM/TMPDIR), and sightmap needs it. Derive the real one.
export HOME="${HOME:-$(getent passwd "$(id -u)" | cut -d: -f6)}"
sightmap browser install >/dev/null
printf '%s\n' "$HOME/.sightmap/browsers" > .chrome-root
echo "chrome ready: $(cat .chrome-root)"
