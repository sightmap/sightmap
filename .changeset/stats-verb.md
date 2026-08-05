---
"@sightmap/sightmap": minor
---

Add `sightmap stats`: corpus totals (views, components, requests, properties, memory entries) plus a per-view component/request table. Counting follows the corpus model — `$refs` are expanded and components dedupe by first-seen name, so the total counts distinct components corpus-wide while each per-view row counts what is reachable in that view after expansion (a global reused by several views appears in each row but once in the totals; view-less globals and requests are in the totals only). `--json` emits a stable machine-readable form (`views` / `components` / `requests` / `properties` / `memory` / `per_view`) suitable for CI consumers.
