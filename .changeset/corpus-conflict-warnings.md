---
"@sightmap/sightmap": patch
---

`validate` now distinguishes **errors** (which fail validation, exit 1) from **warnings** (advisory, exit 0), and warns on silent corpus conflicts that a fallback rule was resolving without telling you:

- **`merge-collision-view`** — two or more views share a `name`.
- **`route-conflict`** — two or more views share the same (normalized) `route`; only the first-declared applies to that URL (this is the "same-route hijack" that can silently drop a view's components).
- **`merge-collision-component`** — two or more root-level global components share a `name` with different selectors.

Findings now carry a stable `code` and `severity`. Scoped component name reuse (the same child under multiple parents) is correctly *not* flagged, since that is intentional. The spec's diagnostic-code table documents the new corpus codes.
