---
"@sightmap/sightmap": minor
---

Add `sightmap.ParseProfileSelector`, which parses a selector against a named selector profile (SEP-0021). The `capture-baseline` profile is the CSS3 subset that every engine in its baseline (Chrome 38, Firefox 29, Safari 9, Edge 80) and matchers re-implemented outside a browser both evaluate identically. Every selector in the profile depends only on an element and its ancestors. A rejection is coded `selector-not-in-profile`, with a message saying why: an engine in the baseline lacks the construct, it depends on more than the element and its ancestors (`:has()`, sibling combinators, positional and state pseudo-classes), or matchers outside a browser commonly evaluate it differently (`*`, `[class]`, `[id]`). An accepted selector comes back split into compounds, each with its text, how narrowly it selects, and the attributes it tests.
