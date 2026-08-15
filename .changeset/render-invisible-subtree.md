---
"@sightmap/sightmap": patch
---

Fix `snapshot`/`capture` rendering an empty component tree when the page root (or any ancestor) is reported invisible. `render.Filter` dropped an entire subtree at the first `IsVisible=false` node, but a zero-size or `visibility:hidden` ancestor — notably the document `<html>` root, which reports height 0 / ignored — can have fully visible descendants, so the whole annotated tree vanished even though coverage still counted the visible nodes. Invisible nodes are now made transparent (the node itself is omitted, visible descendants survive); a genuinely hidden `display:none` subtree still renders nothing, keeping render in lockstep with coverage.
