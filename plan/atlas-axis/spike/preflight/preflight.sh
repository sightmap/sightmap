#!/usr/bin/env bash
# Bot-defense pre-flight (PLAN P6.0): before writing scenarios for a live site,
# record how it treats an automated Chrome — served, soft-blocked, or walled.
# A site that turns us away gets recorded and skipped, never worked around.
#
# Usage: ./preflight.sh [chrome-binary]
# Default: the sightmap-managed Chrome (`sightmap browser install` is idempotent
# and prints the binary path), else $CHROME_BIN.
set -uo pipefail

CHROME="${1:-${CHROME_BIN:-$(sightmap browser install 2>/dev/null | tail -1)}}"
SITES=(
  "https://sightmap.org/"
  "https://www.airbnb.com/"
  "https://www.ikea.com/us/en/"
  "https://www.apple.com/"
  "https://www.ebay.com/"
  "https://www.nike.com/"
  "https://www.amazon.com/"
)
# Interstitial fingerprints seen on the major CDNs' bot walls.
WALL_RE='captcha|are you a human|are you a robot|access denied|pardon our interruption|verify you are|unusual traffic|request blocked|attention required'
# No-egress fingerprint: Chrome rendered its own network-error page instead of
# the site ("errorCode":"ERR_…" in the neterror payload). Seen in sandboxes that
# only route traffic through an HTTP proxy Chrome was not configured for. Also
# caught structurally — identical byte counts across unrelated sites mean you
# are measuring your egress, not the web.
PROXY_RE='"errorCode":"ERR_|main-frame-blocked'

echo "chrome: $CHROME"
command -v "$CHROME" >/dev/null || { echo "chrome binary not found" >&2; exit 1; }

for url in "${SITES[@]}"; do
  out="$(mktemp)"
  if timeout 45 "$CHROME" --headless=new --disable-gpu --no-sandbox \
      --virtual-time-budget=8000 --dump-dom "$url" > "$out" 2>/dev/null; then
    bytes=$(wc -c < "$out")
    title=$(grep -oiE '<title[^>]*>[^<]*' "$out" | head -1 | sed 's/<[^>]*>//')
    if grep -qiE "$PROXY_RE" "$out"; then
      verdict="NO-EGRESS"
    elif grep -qiE "$WALL_RE" "$out"; then
      verdict="WALLED"
    elif [ "$bytes" -lt 5000 ]; then
      verdict="SUSPECT (thin DOM)"
    else
      verdict="SERVED"
    fi
    printf '%-38s %-18s %8s bytes  title: %s\n' "$url" "$verdict" "$bytes" "${title:-<none>}"
  else
    printf '%-38s %-18s\n' "$url" "FAILED (timeout/crash)"
  fi
  rm -f "$out"
done
echo
echo "SERVED = scenario-able today. SUSPECT/WALLED = record it and pick the next fallback (PLAN P6.0)."
echo "NO-EGRESS, or identical byte counts across sites = this environment cannot reach the web"
echo "directly; re-run from a machine with open egress before trusting any verdict above."
