---
"@sightmap/sightmap": patch
---

Fix `browser start --detach` to fail (non-zero exit) when the session is degraded — Chrome's CDP endpoint is alive but the sightmap HTTP daemon has been reaped (e.g. `kill -9`, OOM). The idempotency precheck previously gated success on the CDP probe alone and printed `● already running  server=<port>` for a port it never probed, so scripts proceeded and the next daemon-dependent command (`console`/`network`/`inject`) failed with `reach daemon: connection refused`. It now probes `ServerPort` too and prints a `⚠ degraded` message (mirroring `browser status`) on failure; exit 0 is preserved for the healthy and legacy (no `serverPort`) already-running cases.
