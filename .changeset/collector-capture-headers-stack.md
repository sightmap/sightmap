---
"@sightmap/sightmap": patch
---

The reference network/console collector now populates the faithful-record fields
added for SEP-0005/0006 runtime matching. Observed `Request` records carry
request/response headers (from the CDP `requestWillBeSent`/`responseReceived`
events — cheap, no extra round-trip) and an observed `DurationMs`; observed
exception `Message` records carry the `Stack` (from `exceptionThrown`
`stackTrace.callFrames`, throwing frame first, with 0-based line/column
preserved). Bodies remain fetched on demand. This is what lets
`RequestsForRecord` extract body/header properties and `MessagesForRecord`
resolve stack properties against traffic the reference tooling captured.
