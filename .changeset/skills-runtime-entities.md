---
"@sightmap/sightmap": minor
---

Expand the authoring and browser skills to cover the runtime spec additions.

- **sightmap-authoring**: new "Requests and messages" section documenting `requests:` (route/method), request `properties:` extraction (SEP-0005 `source`/`field`/`pattern`/`transform`), and `messages:` (SEP-0006 `level`/`message` + `source: stack` stack-addressing properties); a new "Runtime activity" step in the per-page loop plus a runtime line in the "Done when" checklist so the loop routes authors to these entities; and view-route-generality guidance (a catch-all route that matches every page is a smell — carve specific routes).
- **sightmap-browser**: reframe the `console`/`network` tooling as the runtime view of the corpus — each record leads with a `[Match]`/`[--]` slot and trails extracted `{name=value}` properties — with the observe-only "reproduce the traffic" note and `--url`-substring / SPA `wait-for` caveats.
- Fix stale command references: `sel-probe -- 'selector'` (the bare `sel-probe 'selector'` form fails) and `gap --include-hidden` (the documented `--visible` flag does not exist).
