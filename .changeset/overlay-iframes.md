---
"@sightmap/sightmap": patch
---

Browser overlay: match components inside iframes. The extension's content script ran only in the top frame, so nothing inside an embedded view (an MCP Apps widget, a sandboxed `about:blank`/`srcdoc` frame) ever matched. It now runs in every frame, including `about:blank` and opaque-origin sandboxed frames, and re-arms its overlay when a host fills a frame with `document.open()`/`write()` after load.
