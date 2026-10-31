---
"@sightmap/sightmap": patch
---

`sightmap lint` now counts compound selectors by their full scope when reconciling the `multi-instance-no-property` rule against captured snapshots. The snapshot count previously matched only the selector's leaf part against every node in the tree, ignoring ancestor combinators, so a selector like `.sidebar div.card` was counted as every `div.card` in the capture — inflating the count and defeating the contract that `count == 1` suppresses the warning and `count == 0` flags a possibly-broken selector. The count now uses the same ancestor-aware NFA matcher as live capture reconciliation, so counts reflect the real (compound) selector's scope. `--snapshot`/`--all-snapshots` and the default `.sightmap/snapshots/**` auto-discovery are unchanged.
