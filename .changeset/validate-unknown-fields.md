---
"@sightmap/sightmap": patch
---

`validate` now warns on unknown fields. The typed loader silently ignored any YAML key it didn't recognize, so a typo like `memroy:` — or a half-baked field — vanished without a trace. `validate` now walks the raw YAML and emits an `unknown-field` **warning** (not an error) for any key the spec doesn't define at its position, at any nesting depth. It warns rather than rejects, so authors can stash experimental fields (e.g. `macros:`) during development; recognized fields — including the reserved tooling fields `access` and `snapshots` — are never flagged, and `.sightmap/config.yaml` is excluded. This completes strict `validate` alongside the earlier required-field and `$ref` checks.
