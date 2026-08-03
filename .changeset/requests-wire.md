---
"@sightmap/sightmap": minor
---

Model `requests:` in the corpus and make it serializable to a stable wire form:

- Global and view-scoped API request definitions (`RequestDef` / `Payload` / `Field`) are now parsed into the corpus — previously `requests:` was known to the schema but silently dropped.
- New `Corpus.RequestsForURL(url, method)` returns every request whose route glob (and optional method) matches the observed request — "all matches apply", per the spec. Reuses the existing route matcher (`:param`, trailing-slash, `**`).
- `Corpus`, `View`, and `RequestDef` now carry JSON tags, so a corpus serializes to a lean wire form (`memory` / `globals` / `views` / `requests`); authoring-only `View` fields (`url` / `access` / `snapshots` / `sourceFile` / `stability`) are excluded. The flattened list is emitted in a stable pre-order (lexical file order, then declaration-order depth-first), locked by a test, so the wire form is reproducible.
