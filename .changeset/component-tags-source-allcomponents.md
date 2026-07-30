---
"@sightmap/sightmap": minor
---

Go library: components can now carry `source:` (relative path to the implementing file — already schema-recognized, previously dropped by the loader) and `tags:` (authored classification labels, e.g. `defect`). Neither is inherited by children, matching `memory`/`properties`/`stability`'s existing convention. `Tags` also flows through `ApplySightmap`'s match result alongside `Memory`. New `Corpus.AllComponents()` returns every component in the corpus (globals plus every view's), deduped by first-seen name — the flat, whole-corpus list a consumer building an upload payload or a lint/coverage report wants, replacing an equivalent hand-rolled loop in `cmd_lint.go`.
