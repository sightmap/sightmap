---
"@sightmap/sightmap": minor
---

Add the SEP-0007 Component/View **state-signal** subset. A corpus-root `signals:` block names a component (present) or a view (route-active) as a named boolean predicate (`{name, ref, tags?}`) — the dependency-free core of SEP-0007. Resolve one with `Corpus.ResolveSignal` / `SignalByName`; validation reports `signal-ref-unresolved` and `signal-ref-ambiguous`. Request/message refs and the temporal/window machinery are intentionally out of scope.
