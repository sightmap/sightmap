---
"@sightmap/sightmap": patch
---

Make `network get` reliable and header-aware. The devtools body endpoint now serves the response/request body eagerly retained on the record (captured at `loadingFinished`) instead of always re-fetching it from Chrome via `getResponseBody`, which failed once the browser had evicted the body ("response body no longer available"). And `network get` now renders the captured request/response headers (they were already on the record from the collector but never surfaced). This closes the HTTP response header/body residual split out of the network-collector gaps.
