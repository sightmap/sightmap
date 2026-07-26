---
"@sightmap/sightmap": patch
---

Fix view route matching so it behaves as the spec documents. Trailing slashes are now normalized off both the route pattern and the URL path before matching, so a slash-free route like `/*/projects` matches the `/acme/projects/` paths that Django, Rails, and many React-Router apps emit — previously such URLs matched no view at all, silently dropping the `[View:]` header, view memory, and view-scoped components. Express-style `:param` segments in view routes now match a single path segment (and score between a literal and `*` for specificity). And `Corpus.ViewForURL` now returns the *most specific* matching view — using declaration order only to break equal-specificity ties — instead of the first match in corpus order, closing a divergence between the exported library and the spec.
