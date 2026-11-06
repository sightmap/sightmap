---
"@sightmap/sightmap": patch
---

Faster component matching on large, deeply nested corpora, with identical results. The matcher now skips an in-flight selector at any node missing an id, exact attribute value, `^=` prefix, class or tag its next compound requires. It keeps in-flight selectors on one shared stack instead of copying them at every node, flattens each compound's attribute tests once per compiled corpus, and reuses walk scratch between calls. On a 54-view, 1,335-definition corpus, matching all 187 captures takes 8x less CPU, and `Matcher.Conflicts` no longer builds a per-node map.
