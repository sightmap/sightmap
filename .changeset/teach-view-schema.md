---
"@sightmap/sightmap": patch
---

`report` and `capture` errors now teach the `views:` file structure instead of naming a field in isolation:

- `report` distinguishes "no views defined at all" from "views exist but none has a `url:`", and both print a minimal `views:` example (with `route:` and `url:` and their roles) — previously it always said "no views with URLs found / Add a url: field", which contradicted `capture`'s route-only advice.
- `capture`'s "no view matches" error shows the same `views:` example and names `route:` explicitly.

The `sightmap-authoring` skill's Phase 1a now shows a complete view-file example (the top-level `views:` list with `version:`/`route:`/`url:`), instead of only telling authors to "create a view file with `route:`" — which invited putting view fields at the file root (silently making it a globals file).
