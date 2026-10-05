---
"@sightmap/sightmap": minor
---

One `extract` object for every property (SEP-0017).

Component, request and message properties now read with `extract: { from, path, pattern, join }`, drawing `from` from one namespace: `dom.text`, `dom.raw_text`, `dom.attr`, `dom.state`, `component`, `component.exists`, `req.body`/`rsp.body`/`req.headers`/`rsp.headers`, and `stack`. Components gain `pattern`, and a path segment written `Name[]` reads every match instead of the first, collapsed into one value by `join`. A bracket with content in a path segment is reserved for future path predicates. `dom.state` reads interactive state separately from the markup attribute of the same name.

The string forms (`extract: text`, `attr=NAME`, `PATH.prop`, `exists:PATH`, and request/message `source`/`field`/`pattern`) still load, lowered to the object, and `validate` warns `extract-legacy-form` with the replacement. They will be removed in a later release. `validate` also warns `extract-privacy-withheld` on a read its component's own `privacy` withholds.

Go API: `ComponentPropertyDef`, `RequestPropertyDef` and `MessagePropertyDef` each carry one `Extract` value in place of their per-entity fields.

The exported corpus JSON (`sightmap export`) changes shape to match: component `extract` becomes an object, and request and message properties carry `extract` in place of `source`/`field`/`pattern`. This SDK still decodes the old shape, but an older reader cannot decode the new one, so upgrade readers before writers.
