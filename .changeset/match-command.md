---
"@sightmap/sightmap": minor
---

New `sightmap match FILE.snap|DIR ...`: one JSON record per capture, matched by its URL as a session pipeline would. Each record gives the view (or the tied views), nodes per component, unmatched regions grouped by nearest hooked ancestor, and nodes claimed by more than one definition: the signals a capture sampler reads to fill gaps and retire stale components.
