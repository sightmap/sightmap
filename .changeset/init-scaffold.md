---
"@sightmap/sightmap": minor
---

Add `sightmap init`: scaffold a schema-correct `.sightmap/` corpus (a commented `components.yaml` and `views/example.yaml` carrying `version: 1` and the top-level `views:` wrapper) so the first files an author sees are already valid instead of written from memory. Existing files are never overwritten, and the scaffolded corpus passes `sightmap validate` as-is.
