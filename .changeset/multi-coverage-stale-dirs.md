---
"@sightmap/sightmap": patch
---

`sightmap multi-coverage` no longer manufactures phantom "global candidate" promotions from stale capture directories. It grouped columns purely by directory name under `snapshots/`, so a leftover or renamed dir (e.g. an old `snapshots/views/` kept alongside the current `snapshots/home/` for the same page) became a second column and made that page's own components look like they "appear in 2+ views" — advising the author to wrongly globalize view-scoped components.

A capture dir is now treated as a real view only when it matches a view in the *current* corpus (by `SnapBasename`). Non-current dirs are still shown in the matrix (marked `*`) for context but are excluded from the cross-view global-candidate analysis, and a warning names them so the author can delete the stale dir or re-capture under the current name. The `sightmap-authoring` skill documents the new behavior.
