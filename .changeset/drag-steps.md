---
"@sightmap/sightmap": patch
---

`sightmap browser drag` now drives pointer-event drags (boards, sortable lists): it scrolls the source into view only when it is not already fully visible, presses inside it, moves in `--steps N` small steps (default 10, about 16 ms apart, button held) and releases at the drop point, instead of jumping straight to the target. New `--to TARGET [--to-position above|below|center]` drops on another component; `--delta-x/--delta-y` still work. The press lands on a point of the source that is not an interactive child, or at `--from-offset X,Y`. Source and target accept component queries as well as probe ids.

`drag` refuses before pressing when the start point is not over the source or the drop point is not over the target (clipped by a scrolling ancestor, covered by a header, or off screen), and names what is in the way.
