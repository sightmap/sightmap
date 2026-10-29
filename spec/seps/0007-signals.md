---
sep: 0007
title: Signals — composing a named, tagged classification from existing entities
author: Clint Ayres (@jurassix)
status: Review
created: 2026-07-31
updated: 2026-10-29
spec-version-target: 1
related-issues: []
related-discussions: []
---

> **Implementation status.** PR #445 shipped the Component/View subset
> (`name`, `ref`, `tags`, read as a current-state boolean). PR #114 adds the
> rest of the declaration and validation layer: `filter:`, `Request` and
> `Message` refs, filter-key resolution on all four kinds, and conformance
> fixture `037-signals`. Evaluation (producing signal events from live
> activity) is specified here but not implemented in the reference SDK.

## Summary

Add a new top-level `signals` concept: a rule that references an existing entity by name (a `Component`, `Request`, `Message`, or `View`; see [SEP-0005](0005-request-properties.md), [SEP-0006](0006-message-entity.md)) and, optionally, filters on that entity's own declared properties, minting a named, tagged classification when it matches. Each match is an event on the session, so a signal can be counted like a metric. A `signals:` rule never redeclares a selector, route, or body pattern of its own — it only composes what the corpus already defines.

## Motivation

A consumer reasoning about a session (or any other stream of runtime activity a sightmap corpus describes) regularly needs to surface a classification that no single existing field captures: a `200 OK` network response whose body says a payment was declined; a component whose extracted text reads "declined" even though its mere presence is expected either way. Today, expressing this means writing bespoke match logic outside the spec entirely, independent of whatever entity (component/request) the classification is actually about — a rule that redeclares its own route glob and body pattern has no structural relationship to the `Request` entity it's actually about, so the two silently drift apart the moment the endpoint's declared route changes.

This SEP proposes the fix directly: a `signals:` rule must reference an entity (`ref:`) and, optionally, filter on that entity's own already-declared properties — never redeclare a selector, route, or body pattern of its own.

## Proposal

### Shape

```yaml
signals:
  - id: <string>                # optional, stable identity (SEP-0020)
    name: <string>              # required, semantic identity of the generated classification
    ref: <entity name>          # required, name of a components:/requests:/messages:/views: entry
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
      - name: outcome
        extract: { from: rsp.body, path: status }
      - name: decline_code
        extract: { from: rsp.body, path: error.code }

signals:
  - name: checkout.payment.declined
    tags: [defect]
    ref: CheckoutPayment
    filter:
      status: 200          # the HTTP status, a reserved identity
      outcome: declined    # a declared property
```

### Semantics

#### `filter:` evaluates against the referenced entity's properties, plus its already-structured identity

For a `Request` ref, `filter:` may name any of its declared `properties:` (SEP-0005), or one of the reserved already-structured identity names (`status`, `method`, `duration`) without any `properties:` declaration. A request's identity is free to filter on; anything read from its body or headers has to be declared as a property first. The following example needs both, ANDed:

```yaml
requests:
  - name: CheckoutRetryPayment
    route: "/api/checkout/pay/retry"
    method: POST
    properties:
      - name: rate_limit_remaining
        extract: { from: rsp.headers, path: X-RateLimit-Remaining }
      - name: outcome
        extract: { from: rsp.body, path: status }

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
        extract: { from: dom.text }

signals:
  - name: checkout.payment.declined.banner
    tags: [defect]
    ref: PaymentBanner
    filter:
      text: Payment declined         # equality on the extracted text, not a glob
```

For a `Message` ref ([SEP-0006](0006-message-entity.md)), `filter:` is often omitted, since a message entity's own `level`/`message` usually identifies the case. When it declares stack `properties:`, a filter may name them:

```yaml
messages:
  - name: CartVersionMismatch
    level: ERROR
    message: cart version mismatch

  - name: ChunkLoadFailed
    level: EXCEPTION
    properties:
      - name: origin_file
        extract: { from: stack, path: top.file, pattern: '([^/]+)$' }

signals:
  - name: cart.abandoned
    tags: [defect]
    ref: CartVersionMismatch
  - name: chunk.vendor.failed
    tags: [defect]
    ref: ChunkLoadFailed
    filter:
      origin_file: vendor.js
```

A `View` ref with no `filter:` fires on every entry to the view. With [SEP-0008](0008-url-properties.md) URL properties, a filter may name the view's declared properties or a `:name` its route binds; a `Request` ref may name its route's bindings the same way:

```yaml
views:
  - name: CheckoutView
    route: "/checkout"
  - name: OrderDetail
    route: "/orders/:order_id"
    properties:
      - name: tab
        extract: { from: url.query, path: tab }

signals:
  - name: checkout.view.active
    tags: [checkout]
    ref: CheckoutView
  - name: orders.returns_tab.viewed
    tags: [returns]
    ref: OrderDetail
    filter:
      tab: returns
```

#### No `filter:` means unconditional

Omitting `filter:` means the rule fires on every match of `ref`. It classifies the same occurrences a static `tags:` field on the entity would ([SEP-0004](0004-component-tags.md)), and additionally names them as events that can be counted. An author reaches for `filter:` only when the entity's mere presence isn't itself the classification.

#### Multiple `filter:` keys are ANDed

`filter: {status: 200, outcome: [queued, deferred]}` requires both — there is no `OR` composition in v1. A classification needing OR semantics across otherwise-unrelated conditions is expressed as multiple `signals:` entries.

#### Multiple matching rules fire independently

If more than one `signals:` rule references the same entity and both match the same live instance, both fire — nothing is deduped or merged into a single result. Each rule is evaluated independently; a consumer sees one classification per firing rule, not one classification with multiple names.

#### What a signal emits

Evaluated over a session, a signal produces **signal events**, points in time, one per occurrence. Counting is the number of events, and a session *has* a signal when it has at least one. This is what lets a signal replace a metric: instead of a consumer deciding which metrics apply to a session, each signal is already an event on every session it occurs in.

What counts as one occurrence depends on the kind `ref` names:

| `ref` kind | One event per | Holds (current state) |
|---|---|---|
| Request | completed request record that matches the request entry (all matches apply) with the filter holding | n/a |
| Message | console or exception record that matches the message entry with the filter holding | n/a |
| View | transition into the state where this view is the page's view (most specific wins) with the filter holding | while that state lasts |
| Component | transition of one element into the state where this component names it ([SEP-0023](0023-most-specific-definition-wins.md)) with the filter holding | while any element is in that state |

Requests and messages are records; views and components are state. Without the distinction, a component signal has no answer to "how many": every DOM mutation, re-render, or observer callback would be a candidate occurrence. Edge-triggering on the state starting to hold gives one event per appearance, per arrival, and per change of a filtered property into a matching value, the same collapse [SEP-0015](0015-component-watch.md) asks of visibility reports. The current-state boolean stays available for completion and availability predicates, which is what the shipped Component/View subset defined.

A component ref follows naming rather than raw selector matches. A ref to a general `Button` does not also count the `DangerButton` a more specific definition names, so the two never double-count one element. `watch` deliberately does not depend on naming, because it generates its own records; a signal reads properties, and properties resolve against the component that names the element.

Each event carries `signal` (the name), `id` (when declared), resolved `tags`, `ref` and `kind`, `ts`, the page's `view` at `ts`, and `properties`: the referenced instance's resolved property values, including route bindings and a request's `status`/`method`/`duration`, minus anything unresolved or withheld by privacy ([SEP-0009](0009-component-privacy.md)). The properties are what make an event countable along dimensions the filter did not fix; `checkout.payment.declined` filters `outcome: declined` and still carries `decline_code`. A stored series SHOULD key on `id` when present, so renaming a signal does not split its history ([SEP-0020](0020-entity-id.md)).

#### Derivation is explicit

A classification produced by a `signals:` match carries the referenced entity's `name` as its link back to what produced it (`CheckoutPayment`, not merely "a network request") — the reference itself, required by this SEP's Shape, is what makes this link structural rather than something a consumer has to reconstruct.

### Conformance

- MUST reject a `signals:` entry whose `ref:` does not resolve to a known `Component`/`Request`/`Message`/`View` name. Diagnostic code: `signal-ref-unresolved` (mirrors the existing `ref-unresolved` code for `$ref`).
- MUST reject (not silently resolve) a `ref:` name that collides across entity kinds (e.g. a `Component` and a `Request` sharing a name) — unlike DOM tree matching, there is no adjacency/tree-position reason to prefer one over the other. Diagnostic code: `signal-ref-ambiguous`.
- MUST evaluate every declared `filter:` key with AND semantics; MUST support both scalar (equality) and array (membership) values per key.
- MUST reject a `filter:` key that does not resolve on the referenced entity, i.e. is not one of its declared `properties:`, not a `:name` its route still binds (`View`, `Request`; SEP-0008), and not a reserved name available for its kind (`status`/`method`/`duration` on a `Request`, `value` on a `Component`). A name declared by several entities of one kind resolves a key any of them declares. Diagnostic code: `signal-filter-unknown`. Without this check the structural link this SEP is built on is unenforced: a filter naming a renamed or never-existent property passes silently and the rule simply never fires.
- MUST reject a `filter:` value that is not a string, integer, or boolean, or a non-empty list of them: a `null`, a float, a mapping, or an empty list. Diagnostic code: `field-type-invalid`.
- MUST reject a `signals:` entry with no `name` or no `ref` (`missing-name`, `missing-ref`), and two entries sharing a `name` (`merge-collision-signal`).
- MUST compare `filter:` values as canonical text, rendering an integer as its decimal value so `0x1F` and `31` agree. MUST reject an unquoted integer that YAML 1.1 and 1.2 parsers read differently (a leading-zero octal like `0200`, or underscores like `1_000`), since two SDKs would otherwise compare different values for one corpus.
- MUST fire a rule with no `filter:` on every match of its `ref:`.
- MUST evaluate each `signals:` rule independently — a live instance matched by more than one rule produces one classification per matching rule.
- MUST NOT require a numeric-comparison filter operator (`>=`, ranges) in v1 — see [Open questions](#open-questions).
- SHOULD surface the referenced entity's `name` as part of the classification's own identity, per [Derivation is explicit](#derivation-is-explicit).
- MUST emit one event per matching record for a `Request` or `Message` ref, and one per transition into the holding state for a `View` or `Component` ref, per [What a signal emits](#what-a-signal-emits). A `Component` ref counts only elements the component names.
- MUST carry `signal`, `tags`, `ref`, `kind` and `ts` on each event; SHOULD carry `id`, `view` and the referenced instance's resolved `properties`.

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
    id: { $ref: "#/$defs/id" }    # SEP-0020
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
      description: "Open-vocabulary classification labels carried onto the signal, unioned with the resolved tags of the entity named by ref. See SEP-0016."
    filter:
      type: object
      description: "Property-name → value (equality) or value-list (membership) constraints, ANDed across keys, evaluated against the referenced entity's declared properties, its route's :name bindings, and its already-structured identity fields. Omitted = unconditional."
      additionalProperties:
        oneOf:
          - { type: [string, integer, boolean] }
          - { type: array, items: { type: [string, integer, boolean] }, minItems: 1 }
```

Note the value types. An earlier draft of this diff allowed only `string`, which made this proposal's own multi-key example above invalid: `status: 200` is an unquoted YAML integer, and the reserved `status` identity genuinely *is* an integer. Integers and booleans are accepted and compared as canonical text, so the natural spelling works. `integer` rather than `number` is deliberate: a float has no single canonical text (`1.50` versus `1.5`), which would become a cross-SDK divergence. `null` and an empty list stay invalid, the first having nothing to compare and the second never matching.

`fileRootFields` gains `"signals"` (already shipped with the subset).

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

Purely additive. Every corpus valid under the shipped Component/View subset stays valid with the same meaning: a `signals:` entry with no `filter:` on a component or view resolves exactly as before. No migration is required.

An SDK that implements only the subset rejects the new shapes: a `filter:` key fails `additionalProperties: false` on `$defs.signal` in the JSON Schema (the Go loader warns `unknown-field`), and a `Request` or `Message` ref is `signal-ref-unresolved`. Release playbook:

1. This SEP merges as Accepted. Its dependencies, [SEP-0005](0005-request-properties.md), [SEP-0006](0006-message-entity.md) and [SEP-0008](0008-url-properties.md), are already Accepted.
2. `sightmap/sightmap` ships `filter:` parsing, `Request`/`Message` refs and filter-key validation in a minor release.
3. Consumers pin `github.com/sightmap/sightmap/go >= <that version>` before authoring `filter:` or request/message refs.

## Open questions

1. **Comparison-operator filters.** `filter:` supports only equality and membership in v1, so "any 5xx" and "slower than 3s" can't be written. A numeric range or comparison operator (`filter: {duration: ">=1000"}`), negation, and pattern matching were considered and deferred to a future SEP, to keep v1's comparison rule (canonical text equality) the same in every SDK.
2. **Composition.** A signal cannot reference another signal, and there is no sequence, absence, or time-window form ("declined, then no retry within 30s"). That is a different kind of rule, evaluated over a sequence of events rather than one instance, and belongs in its own SEP.
3. **Falling edges.** Events mark a view or component state starting to hold. Whether a consumer should also emit the end (so a signal can carry a duration, such as how long an error banner stayed up) is left open; nothing in v1 needs it.
4. **Fan-out policy revisit.** This SEP requires independent firing for multiple matching rules ([Conformance](#conformance)) as the simplest, least-surprising default. A future SEP could introduce a dedup/union mode if real corpora show independent firing producing noise.

## References

- [SEP-0003](0003-component-properties.md) — the DOM `properties:` mechanism a `Component`-referencing `signals:` rule filters against.
- [SEP-0004](0004-component-tags.md) — the static-tag mechanism `signals:` with no `filter:` is a strict superset of.
- [SEP-0005](0005-request-properties.md) — the request-property mechanism a `Request`-referencing `signals:` rule filters against; a direct dependency of this proposal.
- [SEP-0006](0006-message-entity.md) — the console/exception entity a `Message`-referencing `signals:` rule composes; a direct dependency of this proposal.
- [SEP-0008](0008-url-properties.md) — URL properties and route bindings on views and requests, which a filter may name.
- [SEP-0009](0009-component-privacy.md) — privacy, which withholds properties from a signal event.
- [SEP-0015](0015-component-watch.md) — visibility reporting, whose collapse rule component signal events follow.
- [SEP-0016](0016-message-and-signal-tags.md) — how a signal's tags resolve.
- [SEP-0020](0020-entity-id.md) — the stable `id` a stored signal series keys on.
- [SEP-0023](0023-most-specific-definition-wins.md) — which component names an element, and so which a component signal counts.
