---
"@sightmap/sightmap": minor
---

Carry a node's interactive state for `attr=` extraction (SEP-0013).

`attr=checked`, `attr=selected`, `attr=disabled` and `attr=expanded` now read the control's current state as `"true"`/`"false"` (`"mixed"` for an indeterminate checkbox), from native properties and the accessibility tree, instead of the DOM attribute, which records only the initial state. Selector matching still sees the DOM attributes. The browser extension resolves the same state and now supports `raw_text`.
