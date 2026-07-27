---
"@sightmap/sightmap": patch
---

`validate` now surfaces two classes of invalid corpus the loader previously dropped silently (so they shipped with a green check):

- **Missing required fields** — a component with no `selector` (or no `name`), and a view with no `name`, are now errors (`missing-selector` / `missing-name` / `missing-route`) instead of being quietly discarded during load.
- **Unresolved `$ref`** — a `$ref` naming a component that no file defines is now a `ref-unresolved` error (matching the spec's MUST), instead of the reference being silently skipped.

Both exit non-zero. The spec's diagnostic-code table documents the new error codes alongside the corpus-conflict warnings.
