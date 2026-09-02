---
"@sightmap/sightmap": patch
---

`browser eval` now awaits a returned Promise and yields its settled value instead of an opaque `{}`, so async page/tool code (e.g. an awaited `executeTool()`) can be driven directly. A per-eval timeout bounds a never-settling promise. This is a no-op for synchronous expressions.
