---
"@sightmap/sightmap": minor
---

`sightmap network list` now **hides successful static requests by default**, and `--static` shows them. This changes the default output: a script, stylesheet, image, font, media file or manifest that came back 2xx or 304 no longer appears unless you pass `--static`. A failed one (404, 5xx, refused, blocked) still does, as does every Document, XHR, Fetch, EventSource, WebSocket and unknown-type request. This matches `playwright-cli requests`.

Against a dev server such as Vite, every page load fetches hundreds of module scripts. They filled the 1000-entry network buffer and evicted the app's own `/api` calls ("476 earlier network entries dropped"). Successful static requests now go in a separate ring of the same size, so they can fill only that ring. Indexes stay unique across both rings, so `network get N` works for either. The dropped note names the ring: `earlier network entries` for the main one, and `earlier static network entries` only when `--static` is given.

`network list` also takes `--filter REGEXP` (Go syntax, unanchored) to match request URLs, alongside `--url SUBSTR`. Both can be combined with `--static`, `--type`, `--tab` and `--limit`.

Minor, not patch: it adds two flags and changes what an existing command prints by default.
