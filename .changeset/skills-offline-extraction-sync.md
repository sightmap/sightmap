---
"@sightmap/sightmap": patch
---

Docs: sync the authoring and browser agent skills to recent offline-extraction and `eval` changes. The authoring skill now states that every attribute (not just `class`/`id`) matches offline and that `text` falls back to a node's rendered `innerText` when it has no accessible name; the browser skill notes that `browser eval` awaits a returned promise.
