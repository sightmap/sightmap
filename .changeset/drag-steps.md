---
"@sightmap/sightmap": patch
---

`sightmap browser drag` now drives pointer-event drags (boards, sortable lists): it scrolls the source into view, presses inside it, moves in `--steps N` small steps (default 10, about 16 ms apart, button held) and releases at the drop point, instead of jumping straight to the target. New `--to TARGET [--to-position above|below|center]` drops on another component; `--delta-x/--delta-y` still work. The press lands on a point of the source that is not an interactive child, or at `--from-offset X,Y`. Source and target accept component queries as well as probe ids.
