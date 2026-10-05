---
"@sightmap/sightmap": patch
---

Don't surface a blocked node's accessibility value in the rendered tree.

The matcher withholds a property read from a `block` or `mask` node (SEP-0009), but the renderer then substituted the node's accessibility value as `value`, reintroducing the content the matcher had just withheld: a masked input rendered its own value. The fallback is now withheld under `block` and `mask`; declared properties the matcher did surface, such as a `dom.state` read under `mask`, still render.
