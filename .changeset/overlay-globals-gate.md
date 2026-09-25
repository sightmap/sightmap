---
"@sightmap/sightmap": patch
---

Fix the extension overlay rendering nothing on hover for any corpus without a `components.yaml`.

The content script used `state.globals.length` as its "corpus is loaded" signal. But `globals` is the *optional* file-root component list and is `omitempty` on the wire, so a corpus whose components are all view-scoped — the common case, and what `sightmap init` produces — serves no `globals` key at all. The gate was therefore permanently false:

- **Hover was dead.** `onMouseMove` returned before resolving, so `#__sightmap-overlay` injected fine but never got any children: no highlight boxes, no tooltip, no console error. Clicks kept working and logging correct component paths, because the click path gates on `state.version` — which made the failure look like a hover/render bug rather than a readiness-gate bug.
- **The corpus was re-fetched on every poll.** `pollVersion` took the same gate, so it never reached the cheap `/sightmap/version` path and re-downloaded the whole corpus every 4s, logging `[sightmap] loaded N components` each time.

Both paths now share a single `corpusLoaded()` helper keyed on `state.version`, which `fetchSightmap` only records once a load returned a non-empty component set — so the "server answered before its corpus finished loading" retry behavior is unchanged.

The extension manifest version is bumped so an already-installed `~/.sightmap/extension` is re-extracted on the next `browser start` (`ensureExtension` only reinstalls on a version change).
