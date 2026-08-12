---
"@sightmap/sightmap": minor
---

Add runtime matching for console/exception messages. `Corpus.MessagesForRecord`
classifies an observed console record against `messages:` definitions
(case-insensitive level equality + an RE2 `message` regex, precompiled at load),
returning every match so an ambiguous record is surfaced rather than silently
resolved. The `sightmap console` and `sightmap network` devtools listings now
annotate each captured record with the corpus definitions that classify it —
messages via `MessagesForRecord`, requests via route+method identity — leading
each line with the match.
