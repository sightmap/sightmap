---
sep: 0007
title: Signals — composing a named, tagged classification from existing entities
author: Clint Ayres (@jurassix)
status: Draft
created: 2026-07-31
updated: 2026-07-31
spec-version-target: 1
related-issues: []
related-discussions: []
---

## Summary

Add a new top-level `signals` concept: a rule that references an existing entity by name (a `Component`, `Request`, or `Message` — see [SEP-0005](0005-request-properties.md), [SEP-0006](0006-message-entity.md)) and, optionally, filters on that entity's own declared properties, minting a named, tagged classification when it matches. A `signals:` rule never redeclares a selector, route, or body pattern of its own — it only composes what the corpus already defines.

## Motivation

A consumer reasoning about a session (or any other stream of runtime activity a sightmap corpus describes) regularly needs to surface a classification that no single existing field captures: a `200 OK` network response whose body says a payment was declined; a component whose extracted text reads "declined" even though its mere presence is expected either way. Today, expressing this means writing bespoke match logic outside the spec entirely, independent of whatever entity (component/request) the classification is actually about — a rule that redeclares its own route glob and body pattern has no structural relationship to the `Request` entity it's actually about, so the two silently drift apart the moment the endpoint's declared route changes.

This SEP proposes the fix directly: a `signals:` rule must reference an entity (`ref:`) and, optionally, filter on that entity's own already-declared properties — never redeclare a selector, route, or body pattern of its own.

## Proposal

### Shape

```yaml
signals:
  - name: <string>              # required, semantic identity of the generated classification
    ref: <entity name>          # required, name of a components:/requests:/messages: entry
    tags: [<string>, ...]       # optional
    filter:                     # optional; omitted = fires on every match of ref
      <property>: <value>       # equality
      <property>: [<value>...]  # membership (any of)
```

The checkout example from Motivation, under this shape, using the request's declared property from SEP-0005:

```yaml
requests:
  - name: CheckoutPayment
    route: "/api/checkout/pay"
    method: POST
    properties:
      - name: status
        field: "rsp.body.status"

signals:
  - name: checkout.payment.declined
    tags: [defect]
    ref: CheckoutPayment
    filter:
      status: declined
```

### Semantics

#### `filter:` evaluates against the referenced entity's properties, plus its already-structured identity

For a `Request` ref, `filter:` may name any of its declared `properties:` (SEP-0005), or one of the reserved already-structured identity names (`status`, `method`, `duration`) without any `properties:` declaration at all — a real asymmetry worth stating plainly: a request's *identity* is free to filter on, its *extracted content* is not. The following example needs both, ANDed:

```yaml
requests:
  - name: CheckoutRetryPayment
    route: "/api/checkout/pay/retry"
    method: POST
    properties:
      - name: rate_limit_remaining
        field: "rsp.headers.X-RateLimit-Remaining"
      - name: outcome
        field: "rsp.body.status"

signals:
  - name: checkout.payment.throttled_silently
    tags: [defect]
    ref: CheckoutRetryPayment
    filter:
      status: 200                    # already-structured identity — no properties: needed
      rate_limit_remaining: "0"        # declared property, extracted from a response header
      outcome: [queued, deferred]    # declared property, extracted from the response body; membership match
```

For a `Component` ref, `filter:` may name any of its declared `properties:` ([existing DOM extraction](0003-component-properties.md)):

```yaml
components:
  - name: PaymentBanner
    selector: ".payment-status-banner"
    properties:
      - name: text
        extract: text

signals:
  - name: checkout.payment.declined.banner
    tags: [defect]
    ref: PaymentBanner
    filter:
      text: "*declined*"
```

For a `Message` ref ([SEP-0006](0006-message-entity.md)), `filter:` is typically omitted — a message entity's own `level`/`message` usually already fully identifies the case:

```yaml
messages:
  - name: CartVersionMismatch
    level: ERROR
    message: cart version mismatch

signals:
  - name: cart.abandoned
    tags: [defect]
    ref: CartVersionMismatch
```

A `View` ref has no extractable property at all, so a view-referencing signal always omits `filter:` — it scopes a classification to "any activity observed while this view is active," a different kind of statement than a content match:

```yaml
views:
  - name: CheckoutView
    route: "/checkout"

signals:
  - name: checkout.view.active
    tags: [checkout]
    ref: CheckoutView
```

#### No `filter:` means unconditional

Omitting `filter:` means the rule fires on every match of `ref`. This is deliberately the same case a `Component`'s or `Request`'s static `tags:` field already covers ([SEP-0004](0004-component-tags.md)) — referencing an entity with no `filter:` is equivalent to a static tag on it. `signals:` is a strict superset of that mechanism, not a parallel one; an author reaches for `filter:` only when the entity's mere presence isn't itself the classification.

#### Multiple `filter:` keys are ANDed

`filter: {status: 200, outcome: [queued, deferred]}` requires both — there is no `OR` composition in v1. A classification needing OR semantics across otherwise-unrelated conditions is expressed as multiple `signals:` entries.

#### Multiple matching rules fire independently

If more than one `signals:` rule references the same entity and both match the same live instance, both fire — nothing is deduped or merged into a single result. Each rule is evaluated independently; a consumer sees one classification per firing rule, not one classification with multiple names.

#### Derivation is explicit

A classification produced by a `signals:` match carries the referenced entity's `name` as its link back to what produced it (`CheckoutPayment`, not merely "a network request") — the reference itself, required by this SEP's Shape, is what makes this link structural rather than something a consumer has to reconstruct.

### Conformance

- MUST reject a `signals:` entry whose `ref:` does not resolve to a known `Component`/`Request`/`Message`/`View` name. Diagnostic code: `signal-ref-unresolved` (mirrors the existing `ref-unresolved` code for `$ref`).
- MUST reject (not silently resolve) a `ref:` name that collides across entity kinds (e.g. a `Component` and a `Request` sharing a name) — unlike DOM tree matching, there is no adjacency/tree-position reason to prefer one over the other. Diagnostic code: `signal-ref-ambiguous`.
- MUST evaluate every declared `filter:` key with AND semantics; MUST support both scalar (equality) and array (membership) values per key.
- MUST fire a rule with no `filter:` on every match of its `ref:`.
- MUST evaluate each `signals:` rule independently — a live instance matched by more than one rule produces one classification per matching rule.
- MUST NOT require a numeric-comparison filter operator (`>=`, ranges) in v1 — see [Open questions](#open-questions).
- SHOULD surface the referenced entity's `name` as part of the classification's own identity, per [Derivation is explicit](#derivation-is-explicit).

### JSON Schema diff

New top-level property, peer to `views`/`components`/`requests`/`messages`:

```
properties.signals:
  type: array
  items: { $ref: "#/$defs/signal" }
  description: "Rules composing a named, tagged classification from an existing entity's identity and declared properties. See SEP-0007."
```

New `$defs` entry:

```
$defs.signal:
  type: object
  required: [name, ref]
  additionalProperties: false
  properties:
    name:
      type: string
      minLength: 1
      description: "Semantic identity of the generated classification."
    ref:
      type: string
      minLength: 1
      description: "Name of an existing components:/requests:/messages:/views: entry this rule composes. Must resolve; must not be ambiguous across entity kinds."
    tags:
      type: array
      items: { type: string, minLength: 1 }
      description: "Open-vocabulary classification labels carried onto the generated classification. See SEP-0004."
    filter:
      type: object
      description: "Property-name → value (equality) or value-list (membership) constraints, ANDed across keys, evaluated against the referenced entity's declared properties and already-structured identity fields. Omitted = unconditional."
      additionalProperties:
        oneOf:
          - { type: string }
          - { type: array, items: { type: string }, minItems: 1 }
```

`fileRootFields` gains `"signals"`.

## Alternatives considered

### 1. Independent match criteria per rule

A `signals:` rule declares its own `kind`/`method`/`route`/`status`/`body` fields directly, matching signal fields with no reference to any other entity:

```yaml
signals:
  - name: checkout.payment.declined
    tags: [defect]
    kind: network
    method: POST
    route: "/api/checkout/pay"
    rsp_body: '"status"\s*:\s*"declined"'
```

Ruled out: it re-implements matching logic the corpus already does elsewhere, and creates no structural link between the classification and the entity it's actually about — if the referenced endpoint's declared route changes, an independent rule silently drifts out of sync with it, since nothing connects them. The reference-based shape in this proposal makes that link unbreakable by construction: there's no route/selector for the rule to independently duplicate or let drift.

### 2. Mutate the matched entity/instance directly instead of generating a new classification

Instead of producing a new, named classification, apply the rule's `tags:` directly onto the matched request/component instance itself — no new entity in the output, just an annotation on the existing one.

Ruled out: a classification frequently needs to survive independently of how its source instance is later aggregated, deduplicated, or collapsed by a consumer (e.g. a burst of identical retried requests collapsed to one representative in a transcript) — mutating the instance directly means the classification can be silently dropped whenever the instance it rode on isn't the one a consumer's own collapsing logic keeps. A concrete trace: five identical-looking retried requests collapse to one representative entry; if only the third instance's mutation carried the classification, and the first instance is what survives collapsing, the classification vanishes with no error. Producing an independent classification means it collapses (or doesn't) under its own identity, not the source instance's.

## Migration

`signals:` is a new top-level key — purely additive, no existing sightmap corpus references it today. No migration is required for existing sightmaps.

Existing SDKs that encounter a `signals:` key MUST treat it as an unknown top-level field; under `additionalProperties: false` at the root, existing SDKs reject sightmaps that use it until they implement this SEP. Release playbook, matching SEP-0001/SEP-0003's pattern:

1. This SEP merges (status: Accepted) — depends on [SEP-0005](0005-request-properties.md) and [SEP-0006](0006-message-entity.md) also being accepted, since this proposal's `Request`/`Message` examples assume both exist.
2. `sightmap/sightmap` implements `signals:` parsing and `ref:`/`filter:` resolution. Bumped to a minor release.
3. Consumers (e.g. Subtext) pin `github.com/sightmap/sightmap/go >= <new version>` before authoring `signals:`.

## Open questions

1. **Reference key naming.** `ref:` is used throughout this proposal; `signal:` and `from:` were both considered and read worse for a rule referencing a `Component` or `Message` specifically (`signal: PaymentBanner` reads oddly when nothing about `PaymentBanner` is itself a signal). Open to reviewer pushback on the final name.
2. **Should `views:` be referenceable at all in v1?** Included in this proposal (see the `CheckoutView` example) since it costs little and is a natural instance of "no filter = unconditional," but it's the least load-bearing of the four entity kinds — worth confirming reviewers see real demand for it versus deferring.
3. **Comparison-operator filters.** `filter:` supports only equality/membership in v1. A numeric range or comparison operator (`filter: {duration: ">=1000"}`) was considered and deferred — an intrinsic, non-`signals:` mechanism already handles the one case that came up (HTTP status-code ranges) — but a future SEP could add operator support if a real case needs it that intrinsic handling doesn't cover.
4. **Fan-out policy revisit.** This SEP requires independent firing for multiple matching rules ([Conformance](#conformance)) as the simplest, least-surprising default. A future SEP could introduce a dedup/union mode if real corpora show independent firing producing noise.

## References

- [SEP-0003](0003-component-properties.md) — the DOM `properties:` mechanism a `Component`-referencing `signals:` rule filters against.
- [SEP-0004](0004-component-tags.md) — the static-tag mechanism `signals:` with no `filter:` is a strict superset of.
- [SEP-0005](0005-request-properties.md) — the request-property mechanism a `Request`-referencing `signals:` rule filters against; a direct dependency of this proposal.
- [SEP-0006](0006-message-entity.md) — the console/exception entity a `Message`-referencing `signals:` rule composes; a direct dependency of this proposal.
