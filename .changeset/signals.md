---
"@sightmap/sightmap": minor
---

Add a top-level `signals:` entity ([SEP-0007](https://github.com/sightmap/sightmap/blob/main/spec/seps/0007-signals.md)): a rule that references an existing component, request, message, or view by name and optionally filters on its declared properties, minting a named, tagged classification. A signal never redeclares a selector, route, or body pattern, so a classification cannot drift away from the thing it is about.

**Filter values accept an unquoted integer or boolean.** The previous draft's schema allowed only strings, which made SEP-0007's own headline example invalid: `status: 200` is an unquoted YAML integer, and the reserved `status` identity genuinely is one. Values compare as canonical text, and an integer compares as its numeric value in decimal, so `0x1F` and `31` are the same value rather than two different strings. `null`, floats, and empty lists stay invalid.

New validation:

- `signal-filter-unknown` (error) rejects a filter key that does not resolve on the referenced entity. Resolution order is a declared property, then a reserved request identity (`status`/`method`/`duration`), then a component's built-in `value`. A message or view ref accepts no filter keys, since a message's own `level`/`message` already identify it and a view has no extractable property.

  This is what makes SEP-0007's central claim hold. The proposal rests on a signal composing what the corpus already defines so the two cannot drift; without checking the keys, a filter naming a renamed or never-existent property passed silently and the rule simply never fired. The previous conformance fixture did exactly that, filtering on a property it declared nowhere while asserting zero diagnostics.

- `field-type-invalid` (error) now covers filter values, so a bare `key:`, a float, and an empty list are rejected in Go as ajv already rejected them.

`signal-ref-unresolved` and `signal-ref-ambiguous` are unchanged in behavior, but ref and filter resolution now share one entity index, and duplicate message names are caught upstream by `merge-collision-message` — previously two messages sharing a name collapsed to a single entity kind, so the ambiguity check never fired.

Evaluation is not implemented: the SDK parses and validates signals but does not match them against live activity.
