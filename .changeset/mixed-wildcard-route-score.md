---
"@sightmap/sightmap": patch
---

The `sightmap-authoring` skill now warns that a partial-wildcard catch-all route like `/*ViewController` ties an exact route under most-specific-wins (a segment mixing a wildcard with literal text scores as a literal), so it must be declared after the exact views. Matching behavior is unchanged.
