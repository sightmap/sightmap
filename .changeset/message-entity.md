---
"@sightmap/sightmap": minor
---

Add a top-level `messages:` entity ([SEP-0006](https://github.com/sightmap/sightmap/blob/main/spec/seps/0006-message-entity.md)): named console-output and exception patterns, matched by `level` and a `message` regex. This gives console activity what `requests:` gives network activity, a named entity the rest of the corpus can reference by name.

**An uncaught exception arrives as `level: exception`, not `level: error`.** The reference capture emits `log`, `debug`, `info`, `warn`, `error`, and `exception`, so `level: ERROR` matches `console.error` and does not match an exception. SEP-0006 still needs no `kind:` discriminator, because the origin is carried as a level value, but a corpus that wants exceptions has to name them.

New validation:

- `message-regex-invalid` (error) compiles `message` at validation time, matching how component selectors are already checked. The corpus no longer stores a pattern nobody has proven is a pattern. The dialect is pinned to **RE2** (Go `regexp` / the `re2` npm package for JS): a linear-time syntax with no backreferences or lookaround, so authoring-time validation and runtime matching agree across SDKs.
- `merge-collision-message` (warning) reports a duplicated name. This one is load-bearing for SEP-0007: `ref:` resolution counts distinct entity kinds, so two messages sharing a name collapse to one kind and the ambiguity check never fires.
- `message-conflict` (warning) reports two entries that can match the same record, where that overlap is statically decidable: same `level`, identical or absent `message`.
- `message-level-unknown` (lint) catches a level outside the emitted vocabulary. The realistic trap is `WARNING`, CDP's own spelling, which the capture normalizes to `warn`.
- `field-type-invalid` (error) now covers message fields, so `message: 404` and `level: 500` are rejected in Go as ajv already rejected them.

This resolves SEP-0006's open question on ambiguous-match diagnostics, which the SEP delegated to its implementation.

Matching is not implemented: the SDK parses and validates these declarations but does not evaluate them against console records, even though the capture layer already collects both console output and exceptions.
