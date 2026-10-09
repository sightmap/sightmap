---
"@sightmap/sightmap": minor
---

Component `id` and `formerly` (SEP-0020, draft): an optional, opaque identity that is unique among component declarations and stays the same through renames, moves and selector changes, plus a `formerly` list for splits and merges. Neither affects matching. The loader exposes each component's `ID`, `Formerly` and `IDPath` (the ids of its ancestors and itself, which tells a definition's `$ref` placements apart). `validate` reports `component-id-invalid`, `component-id-duplicate` (checked across declarations, so a definition referenced from several places is not a duplicate) and `component-formerly-invalid`; once a corpus declares any id, `lint` reports `component-id-missing` for each declaration without one.
