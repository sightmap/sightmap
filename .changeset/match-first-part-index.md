---
"@sightmap/sightmap": patch
---

Speed up `match.Matcher.Match` and `match.FindAllMatches` on large trees and corpora.

The matcher tried every query at every node and allocated a map sized to the whole query set per node, so cost grew with nodes × queries. Queries are now indexed by a requirement of their first selector part (id, `attr="value"`, class, attribute name, or tag), so a node only tries queries it could start. Per-node maps and per-child ancestor-chain copies are gone, and class attribute selectors (`[class*="..."]` and friends) no longer join the class list on every check. Matching against a 503-query corpus over a 10,000-node tree drops from 290 ms and 466 MB to 6.7 ms and 0.14 MB. Results and their order are unchanged.

Adds `Element.Attr` and `Element.HasAttr`, which resolve an attribute the way selector matching does (`id` and `class` fall back to their dedicated fields).
