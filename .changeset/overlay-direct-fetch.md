---
"@sightmap/sightmap": patch
---

Fix the browser overlay extension rendering **0 components** on pages whose corpus the sightmap server serves correctly. The content script proxied its `/sightmap` fetch through the background service worker, and on a cold/just-woken MV3 worker that round-trip intermittently delivered an empty corpus at page load; the empty result then stuck for the life of the page because the per-instance `version` never changes (so `pollVersion` never refetched) and an open side panel keeps the worker alive. `sightmap snapshot` was unaffected (it reads the corpus in-process over CDP). The content script now fetches the local server **directly** on http-origin pages — `host_permissions` already grants `http://localhost:*` and the server sends `Access-Control-Allow-Origin: *` — using the background proxy only as an https mixed-content fallback, and it treats an empty corpus as "not loaded yet" and keeps retrying instead of caching it.
