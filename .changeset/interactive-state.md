---
"@sightmap/sightmap": minor
---

Carry a node's interactive state for `dom.state` (SEP-0013).

The probe now captures each control's current `checked`, `selected`, `disabled` and `expanded` state from native properties and ARIA, overlaid by the accessibility tree, as `"true"`/`"false"` (`"mixed"` for an indeterminate checkbox). `extract: { from: dom.state, path: checked }` reads it, including where no accessibility tree is available. It is kept apart from the element's attributes, so `dom.attr` and selector matching still see the markup.
