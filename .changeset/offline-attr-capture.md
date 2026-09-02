---
"@sightmap/sightmap": minor
---

The offline element model now captures every attribute the live DOM carries (minus injected sightmap ids and framework-internal `_`-scoping attributes like Angular's `_nghost`/`_ngcontent`), not just the curated subset that landed in the generated selector string. Offline attribute selectors (`[attr=…]`, `[attr*=…]`, …) and `extract: attr=NAME` now match the live DOM for non-standard attributes — e.g. `value` on a `role="option"` `<li>` — closing a source of live/offline divergence.
