---
"@sightmap/sightmap": patch
---

`capture --include-hidden` no longer writes a structurally-duplicate capture on every re-capture of a page with a hidden-only orphan slot (off-screen trays, A/B-hidden variants). The capture-time novelty gate now scores on-disk capture fingerprints under the same visibility filter the live capture used, so a hidden slot already present in the set no longer reads as endlessly novel. `capture-prune` and `capture-novelty` are offline-only and unchanged.
