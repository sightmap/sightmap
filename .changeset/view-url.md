---
"@sightmap/sightmap": patch
---

Fix `report` and make the view `url:` field first-class. `url:` is now read **per view** (with a file-level `url:` as a default for views that omit their own), instead of only at the file level. Previously per-view `url:` was silently dropped, so `report` — which needs a representative URL per view — errored with `no views with URLs found` even when every view declared one. `url:` is also now part of the published schema (SEP-accepted alongside `properties`), so a corpus that declares it validates instead of failing `additionalProperties: false`.
