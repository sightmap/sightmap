---
"@sightmap/sightmap": minor
---

Entity `id` (SEP-0020): every named entity (environments, views, components, `$ref` entries, properties, requests, payload fields, messages, signals and snapshots) accepts an opaque `id` that is unique across the sightmap and stays the same through renames, moves and edits. It doesn't affect matching. The loader exposes each entity's `ID`, and a `$ref` entry's id as `RefID` on the root of its expansion. `validate` reports `id-invalid`, `id-duplicate` (across every entity kind and file; a definition referenced from several places is not a duplicate) and, once a sightmap declares any id, `id-missing` for each entity without one.
