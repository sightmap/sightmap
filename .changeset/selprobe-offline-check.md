---
"@sightmap/sightmap": patch
---

`sel-probe` now cross-checks the offline matcher. It queries the live DOM as before, but also runs the same selector through the offline matcher that `snapshot`/`coverage`/`capture` use, prints that count, and emits a `⚠ offline/live divergence` warning (with a hint) when the two disagree. This closes the false-confidence trap where a selector "verified" against the live DOM is silently dead in the corpus — e.g. attribute selectors on `id` (`[id^="…"]`) match live but never offline, because `id` is a dedicated node field rather than a matchable attribute.
