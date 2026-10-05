---
"@sightmap/sightmap": minor
---

Honor component `privacy` when resolving properties (SEP-0009).

The matcher resolves each node's effective privacy (nearest enclosing declaration wins, `unmask` overrides) and withholds any property value read from a `block` or `mask` node, judged at the node the value is read from, so a `from: component` read cannot surface a value out of a blocked descendant. Under `mask`, `component.exists`, `dom.state`, and `dom.attr` reads of the four interactive-state attributes still resolve. `ComponentMatch` now carries the effective `Privacy` and the component's own `Watch`, and `validate` reports an unknown `privacy` value as `invalid-privacy`.
