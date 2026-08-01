---
"@sightmap/sightmap": patch
---

Teach the corpus schema at the point authors get it wrong, instead of failing silently or generically:

- A **view field at the file root** (e.g. a top-level `route:`/`name:`) now warns that view fields belong under a top-level `views:` list, with a short example, instead of a bare "unknown field".
- A **view-shaped file** that sets `url:` and `components:` but no `views:` now warns that it defines no views and its components are treated as global (previously it validated clean and silently became a globals file).
- A **missing `version:`** now warns (the spec requires `version: 1` in every corpus file).
- `validate` now warns when the **whole corpus has global components but no views** — it can never match a view, so capture, report, and per-view coverage are unavailable.

All are advisory warnings (exit 0); correct corpora stay warning-free.
