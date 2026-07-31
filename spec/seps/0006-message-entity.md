---
sep: 0006
title: A console/exception message entity (`messages[]`)
author: Clint Ayres (@jurassix)
status: Draft
created: 2026-07-31
updated: 2026-07-31
spec-version-target: 1
related-issues: []
related-discussions: []
---

## Summary

Introduce a new top-level `messages` concept, peer to `views`/`components`/`requests`: a named, declarative pattern matching console output and runtime exceptions by `level` and a `message` regex. It gives console/exception activity the same thing `requests:` already gives network activity — a named, referenceable entity.

## Motivation

Every other kind of runtime activity the spec can name has an entity for it: a DOM element has `components:`, a network call has `requests:`. Console output and exceptions have nothing. There is no way today to say, declaratively, "a `cart version mismatch` error means the checkout flow is broken" — a consumer wanting that has to hand-roll its own level/message matching, independently, with no shared vocabulary, and nothing else in the corpus can point at it by name.

## Proposal

### Shape

```yaml
messages:
  - name: CartVersionMismatch
    level: ERROR
    message: cart version mismatch

  - name: SlowNetworkWarning
    level: WARN
    message: 'request .* took over \d+ms'
```

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | yes | A stable identifier for this pattern, addressable by name by other tooling. |
| `level` | string | no | Exact match, case-insensitive, against the console record's level (`ERROR`, `WARN`, `INFO`, …). Match-any if omitted. |
| `message` | string | no | Regex matched against the console record's message text. Match-any if omitted. |
| `description` | string | no | What this pattern means, for a human reading the corpus. |
| `source` | string | no | Relative path to the source most likely to emit this, mirroring `Request.source`/`Component.source`. |

At least one of `level`/`message` should be declared in practice (an entry matching everything isn't useful), but the schema doesn't require it — mirrors `request.method` and `component.selector`'s own optionality where a real corpus might still want to declare a placeholder.

### Semantics

#### One entity, not two ("console" vs "exception")

The obvious first cut of this proposal used a `kind: console | exception` discriminator, splitting the entity by origin. It's dropped: at least one real consumer implementation already models a JS exception as simply an ERROR-level console record, with no second axis to discriminate origin beyond `level` itself, which this proposal already has. A `messages:` entry with `level: ERROR` matches what another implementation might call an "exception"; one with `level: WARN`/`INFO` matches what it might call a plain console message. No `kind` field is needed to express that distinction — see [Alternatives considered #1](#1-a-kind-console--exception-discriminator-field).

#### No extraction mechanism (yet)

Unlike `requests:` ([SEP-0005](0005-request-properties.md)), `messages:` has no `properties:`/extraction mechanism in this proposal. A network endpoint is hit under both good and bad conditions with the same route — extraction exists to tell those instances apart. A console/exception pattern's own `level`+`message` combination usually already fully identifies the case a corpus author cares about; there's rarely a "successful instance" of the exact same message to distinguish from. If a real case needs structured extraction from message text (an error code embedded in a string, say), that's additive future work, not something this SEP blocks.

#### Matching

A live console record matches a `messages:` entry when every declared field (`level`, `message`) agrees; an omitted field matches anything. When more than one `messages:` entry could match the same live record, conformance requires a diagnosable ambiguity, not silent first-match-wins — see [Conformance](#conformance).

### Conformance

- MUST match a live console record against a `messages:` entry using case-insensitive exact match on `level` (when declared) and regex match on `message` (when declared); an omitted field imposes no constraint.
- MUST warn (not silently resolve) when a live record matches more than one `messages:` entry with the same `name` collision potential — mirrors the existing `$ref`/component-name-collision diagnostic discipline.
- MUST NOT require `properties:` support for `messages:` entries in this SEP's scope.
- MAY additionally match `source` for tooling purposes (e.g. surfacing "likely emitted from `src/cart/sync.ts`" in a UI); this SEP does not require any behavior tied to `source` beyond documentary purposes, mirroring `Request.source`/`Component.source`.

### JSON Schema diff

New top-level property, peer to `views`/`components`/`requests`:

```
properties.messages:
  type: array
  items: { $ref: "#/$defs/message" }
  description: "Declarative console/exception patterns, addressable by name."
```

New `$defs` entry:

```
$defs.message:
  type: object
  required: [name]
  additionalProperties: false
  properties:
    name:
      type: string
      minLength: 1
      description: "A stable identifier for this pattern."
    level:
      type: string
      description: "Exact, case-insensitive match against the console record's level. Match-any if omitted."
    message:
      type: string
      description: "Regex matched against the console record's message text. Match-any if omitted."
    description:
      type: string
    source:
      type: string
      description: "Relative path to the source most likely to emit this pattern."
```

`fileRootFields` (the top-level unknown-field allowlist) gains `"messages"`.

## Alternatives considered

### 1. A `kind: console | exception` discriminator field

Split the entity explicitly by origin, with a `kind` field alongside `level`/`message`.

Ruled out — see [Semantics](#semantics): at least one real consumer already collapses both into one console-record concept distinguished by `level` alone. Adding `kind` back would ask every implementation to maintain a distinction that at least one real implementation doesn't have, for no matching behavior it would actually gate. If a future implementation genuinely needs to distinguish uncaught exceptions from explicit console calls at the entity level, that's additive — this SEP doesn't foreclose it, just doesn't require it now.

### 2. Fold into a generalized `events:` supertype that could later absorb `requests:` too

Instead of a new, narrow `messages:` entity, introduce a generic `events:` concept with a `kind:` field selecting `network | console | exception`, unifying what's currently split across `requests:` and this proposal.

Deferred, not rejected. `requests:` is an established, widely-used entity with its own field set (`route`, `method`, `headers`, `Payload`) that doesn't map cleanly onto a console pattern's fields (`level`, `message`) — forcing them under one polymorphic shape now would either bloat every entry with irrelevant optional fields or require a discriminated-union schema this spec hasn't needed anywhere else yet. A narrow, purpose-built `messages:` entity is the smaller, more legible change; generalizing later — once there's a second or third entity kind with the same shape — is the point at which a supertype would actually pay for its complexity.

### 3. Do nothing — leave console/exception patterns unnameable

The status quo: no shared vocabulary for "this recurring console/exception pattern means X," and nothing else in the corpus can point at one by name.

Ruled out per Motivation.

## Migration

`messages:` is a new top-level key — purely additive, no existing corpus references it. No migration required for existing sightmaps.

Existing SDKs that encounter a `messages:` key MUST treat it as an unknown top-level field; under `additionalProperties: false` at the root, existing SDKs reject sightmaps that use it until they implement this SEP — the same constraint every additive top-level concept in this spec carries. Release playbook, matching SEP-0001/SEP-0003/SEP-0005's pattern:

1. This SEP merges (status: Accepted).
2. `sightmap/sightmap` implements `messages:` parsing and matching. Bumped to a minor release.
3. Consumers (e.g. Subtext) pin `github.com/sightmap/sightmap/go >= <new version>` before authoring `messages:`.

## Open questions

1. **Naming.** `messages:` vs. `errors:` vs. something else — `errors:` reads naturally for the ERROR-level case but oddly for an INFO/WARN-level console message that isn't an "error" at all. `messages:` is the more neutral choice made here; open to reviewer pushback.
2. **Should `properties:`/extraction ever be added here?** Flagged as future, non-blocking work in Semantics — worth confirming reviewers agree it's genuinely not needed for v1 rather than a gap being silently deferred.
3. **Ambiguous-match diagnostics.** This SEP requires a diagnosable warning on ambiguous matches (Conformance) but doesn't specify an exact diagnostic code — left for the implementation PR to define, consistent with existing codes like `ref-unresolved`.

## References

- Subtext (a sightmap consumer) models a JS exception as an ERROR-level console record rather than a distinct entity — the real-world precedent behind this proposal's "one entity, not two" call in Semantics.
