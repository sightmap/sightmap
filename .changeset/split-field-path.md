---
"@sightmap/sightmap": minor
---

Body paths accept backslash escapes, and `sightmap.SplitFieldPath` is exported.

A request or message property's `extract.path` into a body is dot-separated. `\.` now addresses a literal dot inside a JSON key and `\\` a literal backslash, so a key named `a.b` is reachable. A path with no backslash resolves exactly as before.

This is a behavior change for a path that already contained a backslash. `a\b` used to address the key `a\b` and now fails to parse; `a\.b` used to mean the key `a\` then `b`, and now means the single key `a.b`. `validate` reports a malformed path as `request-property-path-invalid` rather than letting the property resolve to nothing.

`SplitFieldPath` exposes the same splitting for tools that compile a corpus into another schema.
