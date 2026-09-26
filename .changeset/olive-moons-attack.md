---
"@sightmap/sightmap": patch
---

`Matcher.Conflicts` now deduplicates claims by component **definition** rather
than by name, and `sightmap.Conflict` carries the claiming `Defs` alongside
`Names`.

A component name is unique only within its parent, so two different definitions
can share one. Deduplicating by name silently discarded every conflict between
them — on a production sign-in corpus, five nodes were each claimed by two
definitions and none were reported. Snapshots looked clean while first-match-wins
quietly dropped one claimant on every one of them.

Names alone cannot identify a claimant in that case, so `Conflict.Defs` is
index-aligned with `Names`, and the `[Conflicts]` report disambiguates repeated
names by selector. A definition offering several alternative selectors still
counts once.
