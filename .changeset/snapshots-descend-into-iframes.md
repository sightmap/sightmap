---
"@sightmap/sightmap": minor
---

`snapshot` and every command that reads the page now descend into iframes: same-origin, cross-origin (out-of-process) and nested at any depth, which is how every MCP Apps host renders a widget. Each frame's tree hangs under its `<iframe>` node with bounds in top-page coordinates, and its component ids carry a frame prefix (`2_26`; the top document's stay bare). `click`, `fill`, `hover`, `keypress` (which now takes an optional target to focus), `scroll`, `wait-for`, `bounds` and `screenshot --component` resolve an id to the frame that holds it, and a click's point is computed from the live layout just before it is sent, so it lands after a scroll; a click under an overlay in the page is refused.

A frame is matched as a page of its own, at its own URL: the corpus view whose route matches the frame's URL supplies its components, selectors do not cross the frame boundary, and a masked or blocked `<iframe>` masks or blocks its document. A new `[Frames]` section lists each frame and what was left out.

Each frame is capped at 3000 nodes and a page at 32 frames (`--frame-max-nodes`, `--max-frames`); `--no-frames` restores the old leaf `<iframe>`. `ComponentNode` gains an optional `frameUrl`.
