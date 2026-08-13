---
"@sightmap/sightmap": minor
---

`match.Matcher` gains a chain-matching entry point for consumers that classify a
stream of individual observed elements rather than a full component tree. Each
observed element carries only its own root→leaf ancestor chain, so there was no
first-class API for it — a consumer had to reimplement the spec's component
identity/tag resolution by hand (and route-blind). `MatchChain(chain, pageURL)`
runs the shared NFA matcher over a single-branch spine built from the chain and
returns depth-annotated `ChainMatch` values; `NamesForChain` applies the
nearest-enclosing identity rule and `TagsForChain` the tag-union rule (deduped,
sorted), so the spec's resolution lives once in the shared library. Matching is
route-aware, so view-scoped components apply on the chain exactly as in a
full-tree match. Purely additive packaging over already-exported pieces — no new
matching semantics.
