---
"@sightmap/sightmap": minor
---

Body paths accept backslash escapes, and `sightmap.SplitFieldPath` is exported.

A request or message property's `extract.path` into a body is dot-separated; `\.` now addresses a literal dot inside a JSON key and `\\` a literal backslash, so a key named `a.b` is reachable. Unescaped paths resolve as before. `SplitFieldPath` exposes the same splitting for tools that compile a corpus into another schema.
