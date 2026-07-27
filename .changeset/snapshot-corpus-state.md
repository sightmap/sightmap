---
"@sightmap/sightmap": patch
---

Make `snapshot` loud about corpus-match state instead of silently omitting it. When a corpus is loaded but no view's route matches the URL (e.g. an auth redirect to a login page), the output now shows a `[No view matched] <url>` notice instead of a headerless tree that looks identical to a normal snapshot — so an agent knows the page is off the map. And the `[Coverage]` summary is now printed whenever a corpus is applied, including for a matched view that has no components yet (every interactive node reads as an orphan); previously that case printed only the view header, leaving the documented `snapshot --coverage` bootstrap with nothing to iterate on.
