---
"@sightmap/sightmap": minor
---

Faster matching when one query set is walked over many trees, with identical results. `match.Prepare` indexes a compiled query set once and `Queries.FindAll` walks any number of trees with it, where `FindAllMatches` re-indexed on every call; `Matcher.MatchConflicts` answers `Match` and `Conflicts` in one traversal instead of two; and the specificity memo is a `sync.Map`, so parallel matches no longer queue on a lock. On a 292-capture, 1,381-definition corpus, a selector derivation that matches every capture dozens of times spends several seconds less in the matcher.
