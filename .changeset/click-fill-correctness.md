---
"@sightmap/sightmap": patch
---

Make `browser click` and `fill` fail loudly instead of silently no-op'ing. `click` now scrolls the target to the center of the viewport and verifies its center is the top-most element there before dispatching — it errors when the target can't be positioned in the viewport (previously an off-screen target below the fold was clicked at a coordinate that hit nothing and still exited 0) or is covered by another element (an open overlay/modal). The success confirmation reports the real post-scroll coordinates. `fill` now reads the value back after typing and errors when a non-empty value was typed but the field is still empty — the signature of a React-controlled input where plain typing doesn't stick — telling you to retry with `--clear`.
