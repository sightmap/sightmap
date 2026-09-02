---
"@sightmap/sightmap": minor
---

Add `browser inject` for persistent script injection. Unlike `eval` (which runs once in the current document), `inject --persist` registers a script with the running session daemon, which re-applies it at the start of every new document in every tab via CDP `Page.addScriptToEvaluateOnNewDocument` — so it survives navigations and new tabs for the life of the session. `--file` loads the source from disk, and `--list` / `--remove ID` manage the persisted set. Useful for polyfills, overlays, and debugging/experimentation bundles that must outlive a multi-page flow.
