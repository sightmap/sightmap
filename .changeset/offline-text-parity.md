---
"@sightmap/sightmap": minor
---

Offline `text` extraction now falls back to a node's rendered text when it has no accessible name, so `extract: text` on role-less elements (custom elements, bare `<span>`s) resolves the visible value instead of silently empty. Component text is captured from `innerText` — rendered, transform- and visibility-aware, excluding `<style>`/`<script>` and non-rendered nodes — and normalized to a single clean shape (whitespace runs collapsed, ends trimmed) at the build boundary, so `snapshot`/`coverage`/`bounds` and library consumers all see the same value the runtime does. Adds a `text` field to the component-node schema.
