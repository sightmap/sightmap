---
"@sightmap/sightmap": minor
---

**Breaking:** split the matching engine out of `Corpus`. `Corpus` is now pure, serializable data; build a `Matcher` with `NewMatcher(corpus)` to match a live component tree. `Corpus.MatchTree` and `Corpus.Components` move to `Matcher.MatchTree` / `Matcher.Components` (the per-URL compiled-query cache now lives on the `Matcher`). Read-only corpus queries (`ComponentsForURL`, `ViewForURL`, `RequestsForURL`, `GlobalComponentNames`) stay on `Corpus`.
