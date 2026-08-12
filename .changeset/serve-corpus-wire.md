---
"@sightmap/sightmap": patch
---

`sightmap serve` and the `browser start` daemon now serve the canonical Corpus
wire — the same shape library consumers see (`selectors[]` arrays, components
nested under each view) — inside a thin `{site, version, corpus}` envelope,
replacing the bespoke pre-compiled shape. The bundled browser extension consumes
it directly; cache-busting via `/sightmap/version` is unchanged.
