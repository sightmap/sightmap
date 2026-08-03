---
"@sightmap/sightmap": minor
---

**Breaking:** rename the rule-side corpus types to the `Def` convention — `match.SightmapComponent` → `match.ComponentDef` and `match.SightmapMatch` → `match.ComponentMatch`. A `ComponentDef` describes what to match; a `ComponentMatch` is the result of matching one. No behaviour change; callers update to the new names.
