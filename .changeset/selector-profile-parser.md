---
"@sightmap/sightmap": minor
---

Add `sightmap.ParseProfileSelector`, which parses a selector against a named selector profile (SEP-0018). The `capture-baseline` profile is the CSS3 subset every engine in its baseline (Chrome 38, Firefox 29, Safari 9, Edge 80) implements identically. Each rejection is coded `selector-not-chain-evaluable` (the match depends on more than the element and its ancestors: `:has()`, sibling combinators, positional and state pseudo-classes) or `selector-not-in-profile`. An accepted selector comes back split into compounds, each with its text, how narrowly it selects, and the attributes it tests.
