---
"@sightmap/sightmap": patch
---

Fix `capture-novelty` reporting a genuinely-novel capture as redundant when the candidate path's spelling (absolute vs relative, or `./`-prefixed) didn't lexically match the on-disk spelling discovery emits. `viewset.ViewSlots` now excludes the candidate by identity (canonical `filepath.Abs` → `filepath.Clean` → `filepath.ToSlash` on both sides) rather than a raw string compare, so the candidate is dropped from "others" regardless of how the operator spells it. The header count (`vs N existing capture(s)`) and verdict are now spelling-invariant; the live capture gate (`excludePath == ""`) and `capture-prune` are unchanged.
