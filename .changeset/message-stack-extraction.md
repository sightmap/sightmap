---
"@sightmap/sightmap": minor
---

`messages:` can now match and extract on exception **stack traces** (a SEP-0006
follow-on). An observed `Message` gains an optional `Stack []Frame` (throwing
frame first; `Frame{Function, File, Line, Column}`), empty for a plain console
record. A `MessageDef` gains a stack-addressing `properties[]` mirroring
SEP-0005's `source`/`field`/`pattern`/`transform`: the one source is `stack`,
`field` names a frame and attribute (`top.file`, `top.function`, `1.line`, where
`top` aliases frame `0`), an optional RE2 `pattern` refines the resolved value,
and `transform` post-processes it. `Corpus.MessagesForRecord` now folds the
extracted values into each `MessageMatch.Properties`, bringing it to parity with
`ComponentMatch`/`RequestMatch`; unresolved values are omitted silently, so a
plain console record (no stack) simply extracts nothing. The reference CLI
validates the new declarations (`message-property-invalid-name` /
`-source-invalid` / `-no-field` / `-pattern-invalid`).
