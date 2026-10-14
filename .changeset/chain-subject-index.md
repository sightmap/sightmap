---
"@sightmap/sightmap": patch
---

`PrivacyForChain` and `WatchedForChain` index privacy and watch rules by subject and resolve each chain element bottom-up from its own ancestors, without allocating per call beyond the result (and, for `WatchedForChain`, one name list per watched element).
