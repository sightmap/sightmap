---
"@sightmap/sightmap": patch
---

Two authoring-clarity fixes for the offline inventory and the capture gate.

`sightmap stats` now attributes corpus-root **global** components and requests to their own `(global) · (all views)` row instead of showing `0` against every view. A corpus whose coverage lives entirely in globals (e.g. a single-view app mapped with global components) previously rendered a per-view table that read as empty, with the confusing footer "per-view rows sum to 0". The globals are shown once, on a leading row, and the footer explains them — globals apply to every view, so folding them into one view's row would misattribute coverage. The `--json` contract is unchanged.

`sightmap capture`'s novelty-gate message no longer reads like it is refusing a first baseline. The first capture of a view always writes (an empty set can't be redundant); reaching the gate means a baseline already exists and the new capture adds nothing, so the message now says so plainly ("<view> already has N capture(s); this one adds no new component or interactive slot — not saved"). The `sightmap-authoring` skill is updated to match and to reinforce the first-capture-always-writes guarantee.
