---
"@sightmap/sightmap": minor
---

`match.ClaimCache`: matching the same trees against a run of similar corpora walks each tree only for selectors it has not met. `Matcher.MatchConflictsCached` / `ConflictsCached` return exactly what `MatchConflicts` / `Conflicts` do; `CachedQueries.Claims` / `FindAll` are `FindAll` from the cache. `MatchQuery` gains `Key`, the selector text.
