---
"@sightmap/sightmap": minor
---

Snapshots and captures record the page URL. The header of a `.snap` now carries a `url:` line after `route:`, and `viewset.URLOf` reads it back, so an offline consumer can re-derive which view a capture belongs to (or match it against routes of its own) without the live page. Captures written earlier have no `url:` line; `URLOf` returns "" for them.
