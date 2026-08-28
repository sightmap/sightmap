---
"@sightmap/sightmap": minor
---

Component properties now resolve offline over the component tree (SEP-0010). `Matcher.Match` populates each `ComponentMatch.Properties` from the matched tree with no live DOM, so a downstream consumer holding a serialized component tree gets property values directly. The `extract` grammar is the four tree-closed forms — `text`, `attr=NAME`, `PATH.prop`, `exists:PATH` — and `transform` is removed from component properties. `sightmap validate` now checks component `properties[]`: duplicate names within a component and unrecognized/removed extract modes are errors.
