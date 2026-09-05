---
"@sightmap/sightmap": patch
---

Fix `browser wait-for --component 'X'` timing out whenever the query matched more than one node on the page. The wait polled with the strict single-node resolver (the one `click`/`fill`/`hover` need), so a "several match" result was reported as an ambiguity error and mistaken for "not present yet" — even though `snapshot` listed every instance and `wait-for --selector` with the same multi-match CSS returned immediately. `wait-for --component` now treats presence of ≥1 match as satisfied (multiple matches are proof the component is on the page, which is exactly what the caller is waiting for), via a new `compquery.Present` presence predicate. The interaction commands are unchanged: they still resolve to a single target through the strict `compquery.Resolve`. Narrowed queries keep working — a property predicate (`X[status="Published"]`) or an occurrence index (`X#0`) still matches, and `X#N` waits until occurrence N exists.
