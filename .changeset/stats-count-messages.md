---
"@sightmap/sightmap": patch
---

`sightmap stats` now reports a **Messages** count. The offline inventory listed views, components, requests, properties, and memory but silently omitted the SEP-0006 `messages:` entity, so a corpus's console/exception matchers were invisible in the totals (and in `--json`). Adds `messages` to the totals table and the `--json` contract.
