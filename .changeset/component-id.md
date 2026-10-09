---
"@sightmap/sightmap": minor
---

Component `id` (SEP-0020, draft): an optional, opaque identity that is unique among component declarations and stays the same through renames, moves and selector changes. It doesn't affect matching. The loader exposes each component's `ID` and `IDPath` (its id, prefixed inside a `$ref` expansion by the component holding the `$ref`: it tells a definition's placements apart and doesn't change when a component moves). `validate` reports `component-id-invalid` and `component-id-duplicate` (checked across declarations, so a definition referenced from several places is not a duplicate); once a corpus declares any id, `lint` reports `component-id-missing` for each declaration without one.
