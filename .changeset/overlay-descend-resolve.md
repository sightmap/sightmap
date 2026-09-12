---
"@sightmap/sightmap": patch
---

Overlay: resolve components when hover lands on a wrapper above the component (e.g. a `pointer-events:none` control).

The overlay extension resolved the hovered element bottom-up with `closest()`. When a control has `pointer-events:none` and a styling wrapper sits ABOVE the component container (as JetBlue's checkout form fields do — a `.first-name` div wraps `jb-form-field-container > input`), the hit-test lands on that wrapper, which is an *ancestor* of the component subtree, so `closest()` walks up and finds nothing — the field appeared uncovered even though the CLI resolver (top-down) matched it. On a resolution miss the overlay now descends into the hovered element to the deepest active-component element whose box contains the pointer, and resolves from there.
