---
"@sightmap/sightmap": patch
---

Fix `browser start` silently running sessionless when it couldn't persist the session file. `start` now creates the corpus dir if absent (so the session lives at the per-corpus `.sightmap/.session` rather than a shared `$TMPDIR` fallback) and fails loudly if it still can't write it, instead of continuing without a session. Client commands that find no session file now warn before falling back to the default CDP port — where a session may belong to a different corpus or agent — rather than silently attaching to a foreign tab. Applies to both the launched and `--attach` start paths.
