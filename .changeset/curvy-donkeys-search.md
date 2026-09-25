---
"@sightmap/sightmap": patch
---

Extension: address components by their full path, not their leaf name.

Component names are unique only *within a parent* — that scoping is the whole
point of `children:`. The overlay treated them as globally unique in three
places, and on any real page that silently produced wrong answers:

- `activeComponents()` deduplicated the corpus by name, so a production sign-in
  map of 104 components collapsed to 54. Clicking a text input resolved two
  levels deep instead of six, because the surviving `Form` was a different form.
- Parent scoping in `resolveElement()` accepted an unrelated namesake's match as
  a component's parent.
- `PATH.prop` extraction resolved each path segment by name across the whole
  corpus, so a submit button's `Label.label` found a text field's label, matched
  nothing inside the button, and dropped the property.

All three now key on a component's address (its `parentChain` plus its name).
`resolveElement` visits components ancestor-first and requires a component's own
parent to have matched, and each returned match carries its `address`.
