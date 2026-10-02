---
"@sightmap/sightmap": minor
---

Honor component `privacy` when resolving properties (SEP-0009).

The matcher resolves each node's effective privacy (nearest enclosing declaration wins, `unmask` overrides) and withholds any property value read from a `block` or `mask` node, judged at the node the value is read from, so `PATH.prop` cannot surface a value out of a blocked descendant. Under `mask`, `exists:` and the interactive-state attributes still resolve. `ComponentMatch` now carries the effective `Privacy` and the component's own `Watch`, and `validate` reports an unknown `privacy` value as `invalid-privacy`.
