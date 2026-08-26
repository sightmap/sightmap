---
"@sightmap/sightmap": patch
---

`browser bounds` now accepts the full component-query grammar — property predicates and descendant chains — the same engine as `click`/`fill`/`hover`/`wait-for`. Previously it matched component names only, so `bounds 'Card[title="X"]'` or `bounds 'Row Star'` returned "matched no component". Multi-match is preserved (a query returns every matching component's box), `--substring` still does name-only fuzzy matching, and `--all` is unchanged. Also documented the existing `wait-for --component` and `--view` flags in `browser --help`.
