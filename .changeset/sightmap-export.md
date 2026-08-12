---
"@sightmap/sightmap": minor
---

Add `sightmap export [dir]`: load a `.sightmap/` corpus and emit the canonical
Corpus wire — the exact `json.Marshal(sightmap.Corpus)` shape a library consumer
reads (`selectors[]` arrays, components nested under each view, plus `requests`
and `messages`) — to stdout, a file (`-o FILE`), or an HTTP endpoint (`--url`,
POSTed as `application/json` with no auth headers). The `.sightmap/` directory is
auto-detected by walking up from `[dir]` or the cwd, and TLS verification is
skipped for local hosts (`localhost`, `127.0.0.1`, `.test`) or under an explicit
`--insecure`. A companion `sightmap push URL [FILE]` POSTs a corpus JSON (from a
file or stdin) through the same transport.

This replaces the hand-rolled Python collector (`collect_and_upload_sightmap.py`)
that shipped a second, lossy serializer — it flattened views into compound
selectors and dropped routes, requests, and messages. Routing the upload through
the Go loader makes it the single source of truth and shares the exact
`sightmap.Corpus` type with the server-side reader, so the two ends cannot drift.
