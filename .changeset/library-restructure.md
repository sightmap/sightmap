---
"@sightmap/sightmap": minor
---

Restructure the Go library for downstream consumers. All corpus vocabulary,
observed runtime records, and match-result types now live in a single
self-contained `sightmap` package, with the matching engines consolidated behind
one `match.Matcher`. Types follow a consistent naming model — `…Def` for a spec,
bare structs for observed records, `…Match` for results — and extracted values
share a typed `PropertyValue`. This is a breaking change to the Go import surface
for library consumers (there were none before this release).
