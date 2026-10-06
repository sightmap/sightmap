---
"@sightmap/sightmap": minor
---

Resolve privacy and watch from every component whose selector matches an element, as SEP-0009 and SEP-0015 now state, instead of from whichever component names it. A broader component naming an element (a `Field` child naming every `input` in a form) no longer discards a narrower component's `block`, a view component sharing a global's name no longer drops the global's privacy or watch, two declarations on one element resolve to the strictest, and `block` is absolute: an `unmask` reopens a `mask`, never a `block`.

`ComponentMatch.Watched` lists every watched component matching a node, and `ComponentMatch.Watch` reports whether any does. New on `match.Matcher`: `Privacy` and `Watched` resolve every node of a tree, including nodes no component names. `match.Withholds` is exported, so a consumer withholding properties it extracted itself applies the matcher's rule.
