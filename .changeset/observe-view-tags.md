---
"@sightmap/sightmap": minor
---

Show a page's view tags in `observe` (SEP-0004).

The view header now prints a `tags:` line: the union of tags across every view whose route matches the page, so a broad tagged view is not shadowed by the narrower view that wins identity.
