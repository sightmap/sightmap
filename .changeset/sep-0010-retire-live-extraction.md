---
"@sightmap/sightmap": minor
---

Retire the live-DOM component-property extraction pass and remove `transform` from request and message properties (SEP-0010). Component property values now come solely from the matcher's offline, tree-closed resolution — the CDP/JS extraction (`observe.ExtractProperties` and its `properties.js`) is gone, and `snapshot`/`bounds`/`query` read the values the matcher already resolved. Request and message `properties[]` keep `source`/`field`/`pattern`; fold any post-processing into `pattern` (RE2 + capture groups). `ApplyTransform` is removed.
