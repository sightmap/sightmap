---
"@sightmap/sightmap": patch
---

`sightmap browser click` no longer scrolls a target that is already wholly on screen and unobscured, and scrolls an off-screen one into view only as far as needed horizontally. Centering it scrolled every scrollable ancestor, including an `overflow: hidden` one widened only by an open tooltip; the tooltip then closed, the scroll snapped back, and a repeated click on the same control could land on its neighbour.
