---
"@sightmap/sightmap": patch
---

Skills: teach the fields added in 0.34.0.

`sightmap-authoring` now covers `privacy`, `watch` and `tags` on components, URL properties on views and requests, and file-root `environments` and `origins`. It also corrects the message guidance: an uncaught exception arrives with level `exception`, so `level: ERROR` does not match it. `sightmap-browser` explains the view `tags:` line in a snapshot, and why a component under `privacy: block` or `mask` renders without its content properties.
