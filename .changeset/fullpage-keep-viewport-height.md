---
"@sightmap/sightmap": patch
---

A full-page screenshot (`capture --screenshot`, `browser screenshot --full-page`) no longer stretches `vh`-sized elements. The viewport was pinned to the document's height, so `vh`, `svh` and `dvh` lengths resolved against the document: a `72vh` hero drew 898 px taller than the box the probe measured, and everything under it was drawn that far below its box (a `position: fixed; bottom: 0` bar drew at the document's end). Only the width is pinned now, to keep the page at the layout the probe saw; the whole document is still captured.
