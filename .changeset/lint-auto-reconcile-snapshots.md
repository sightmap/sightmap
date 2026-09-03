---
"@sightmap/sightmap": patch
---

`sightmap lint` now auto-reconciles the `multi-instance-no-property` rule against captured snapshots by default. The static heuristic guesses from selector shape alone and false-positived on container-ish selectors that in fact match a single node; when captures exist under `.sightmap/snapshots/`, lint now uses their real match counts (a count of 1 suppresses the warning) without needing an explicit `--all-snapshots`. With no captures present it runs the static heuristics unchanged, and `--snapshot`/`--all-snapshots` still control the set explicitly. A missing `snapshots/` directory is no longer an error.
