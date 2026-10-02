---
"@sightmap/sightmap": minor
---

Add `tags` to messages, ratify `tags` on signals, and close the `view.tags` gap (SEP-0016, Review).

`tags` now reaches every matchable entity. SEP-0004 deferred messages because console and exception events "aren't a matchable entity in the spec today"; SEP-0006 then created the `messages` entity, matched by `level` and a `message` regex, so that premise no longer holds and no separate mechanism is needed.

Message tags resolve as a union across every matching entry, which matters more here than anywhere else: message *identity* is a refusal rather than a winner, since SEP-0006 requires a consumer to surface an ambiguity when a record matches several entries. The classification survives that ambiguity even though the name does not.

`signal.tags` already shipped via SEP-0007 with a schema description citing a SEP that never mentions signals. It is now documented, and its resolution defined: a signal's effective tags are its own unioned with the resolved tags of the entity its `ref` names.

Also closes the long-standing `view.tags` divergence. It was in the JSON Schema but absent from `ViewDef` and the Go loader's allowlist, so a corpus carrying a view tag validated under ajv and was rejected by Go. New `Corpus.TagsForURL` resolves view tags as a union across every matching view, and `Corpus.TagsForSignal` does the same for signals.
