---
"@sightmap/sightmap": patch
---

Make browser-use feedback loud: the interaction commands (`click`, `fill`, `hover`, `keypress`, `scroll`, `drag`, `wait-for`, `dialog`, `tabs resize`) now echo a short confirmation on success instead of exiting silently — e.g. `clicked CartNavButton @ (674,30)` — so an agent driving the browser can see what happened. Several raw CDP/Go errors are now rewritten into actionable messages: a `wait-for` timeout reads `timed out after 800ms waiting for selector "..."` (not `context deadline exceeded`), resolving a dialog when none is open says so plainly, and fetching an evicted network response body explains that bodies are only retained briefly. `snapshot` now prints a note when no `.sightmap` corpus is found at the target directory instead of silently rendering an un-annotated tree.
