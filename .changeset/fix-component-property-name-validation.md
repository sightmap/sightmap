---
"@sightmap/sightmap": patch
---

Fix `sightmap validate` to reject `componentProperty.name` values the JSON Schema rejects. Component properties were the only property-bearing entity missing both the YAML-tag check (`field-type-invalid`) and the name-pattern check (`component-property-invalid-name`); request and message properties already enforced both. A corpus with `name: true` (bool tag, lexeme `"true"` matches `^[a-z][a-z0-9_]*$`), `name: "5"` (string tag, content fails the pattern), `name: 5`, or `name:` now fails `sightmap validate` with a non-zero exit, matching `spec/scripts/validate-sightmap.mjs` (ajv). Notably, a bool-named property is reachable downstream via `PATH.prop` resolution (e.g. `Header.true`), so the tag check closes a real wire into `ComponentMatch.Properties`, not just an exit-code divergence.
