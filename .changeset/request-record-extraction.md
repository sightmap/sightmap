---
"@sightmap/sightmap": minor
---

Observed `Request` records can now carry the payload a `RequestDef`'s
`properties[]` addresses, and a new matcher resolves them — the runtime half of
SEP-0005. `Request` gains optional (omitempty) `DurationMs`, `ReqHeaders` /
`RspHeaders` (`[]Header`), and `ReqBody` / `RspBody` (`*Body`) fields, so a
producer holding a full capture can populate them while a lazy producer leaves
them nil and the wire stays lean. `Corpus.RequestsForRecord(rec) []RequestMatch`
does route+method identity matching (via `RequestsForURL`) and then resolves each
matched def's `properties[]` against the record — `source` → `field` (JSON
dot-path with numeric array indexing for a `*.body`; case-insensitive header
lookup for a `*.headers`) → optional RE2 `pattern` (capture group 1 else the whole
match, or a scan of the raw source when `field` is absent) → optional
`transform`. Unresolved values are omitted silently per the spec, so an
incomplete record degrades to fewer properties rather than erroring.
