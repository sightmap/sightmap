---
"@sightmap/sightmap": patch
---

Offline selector matching now matches the live DOM for `id`, `class`, and SVG. Three gaps are closed so a selector `sel-probe` verifies live behaves the same way `snapshot`/`coverage`/`capture` see it offline:

- **Attribute selectors on `id`** (`[id^="issue_"]`, `[id$=…]`, …) now match. `id` lives in a dedicated node field, so the matcher resolves `id`/`class` attribute selectors to those fields — not only to captured `attrs`.
- **`placeholder`** is captured, so `input[placeholder="…"]` matches offline.
- **SVG classes** are captured. On SVG elements `className` is an `SVGAnimatedString`, which broke the old extraction (it threw and dropped the whole selector); `probe.js` now uses `classList`, so `svg.lucide`, `[class*="lucide"]`, and `:has(svg.lucide-x)` match offline.

The authoring skill and docs are corrected accordingly: attribute selectors on `class` and `id` are no longer described as "don't work offline" — they work, for HTML and SVG alike.
