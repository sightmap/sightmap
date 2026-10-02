---
"@sightmap/sightmap": minor
---

One `extract` object for every property (SEP-0017).

Component, request and message properties now read with `extract: { from, path, pattern, join }`, drawing `from` from one namespace: `dom.text`, `dom.raw_text`, `dom.attr`, `dom.state`, `component`, `component.exists`, `req.body`/`rsp.body`/`req.headers`/`rsp.headers`, and `stack`. Components gain `pattern` and `join`, and `dom.state` reads interactive state separately from the markup attribute of the same name.

The string forms (`extract: text`, `attr=NAME`, `PATH.prop`, `exists:PATH`, and request/message `source`/`field`/`pattern`) still load, lowered to the object, and `validate` warns `extract-legacy-form` with the replacement. They will be removed in a later release. `validate` also warns `extract-privacy-withheld` on a read its component's own `privacy` withholds.

Go API: `ComponentPropertyDef`, `RequestPropertyDef` and `MessagePropertyDef` each carry one `Extract` value in place of their per-entity fields.
