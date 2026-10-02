---
"@sightmap/sightmap": minor
---

Resolve a console record's tags as a union (SEP-0016).

`MessageMatch` now carries each matched message's `tags`, and `Corpus.TagsForRecord` returns the union across every message a record matches, deduplicated and sorted. The union holds even when the record matches several messages and its identity is ambiguous.
