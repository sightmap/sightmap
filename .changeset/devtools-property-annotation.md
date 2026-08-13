---
"@sightmap/sightmap": minor
---

The `browser start` devtools now surface extracted property values, not just
matched-def names. The collector eagerly retains request/response bodies for
XHR/Fetch traffic (request bodies inline from `requestWillBeSent`; response
bodies fetched on `Network.loadingFinished`, off the event loop), so a buffered
`Request` record is complete. `sightmap network list|get` annotate via
`RequestsForRecord` — resolving each matched request's `properties[]` against the
live headers/body — and `sightmap console list|get` surface a matched message's
stack `properties[]`. Extracted values render as a trailing `{name=value, …}`
token on list lines and a `Properties:` block in `get` detail, e.g.
`GET /api/checkout/pay → 200 OK (Fetch) {outcome=declined}` — the "200 OK but the
body says declined" case, now visible.
