---
sep: 0004
title: Authored classification tags via `tags[]`
author: Clint Ayres (@jurassix)
status: Draft
created: 2026-07-29
updated: 2026-07-29
spec-version-target: 1
related-issues: []
related-discussions: []
---

## Summary

Add an optional `tags: string[]` field to `Component`, `View`, and `Request` entries. A tag
is an open-vocabulary classification label — `defect`, `ui-risk`, whatever an author needs —
distinct from a component's `name` or a view/request's identity. Where identity answers
"what is this," `tags` answers "does this belong to some cross-cutting classification I
care about." Each of these three entity types already has its own rule for resolving
*identity* when more than one definition could apply to the same match (nearest-enclosing
wins for components, most-specific-route wins for views, all-matches-apply for requests).
Tags deliberately do **not** follow whichever of those rules applies — they resolve as a
**union across every applicable definition**, so a broad, tagged definition is never
shadowed by a narrower, untagged one that happens to win identity.

## Motivation

A sightmap today can name things but cannot classify them. Consider a checkout flow:

```yaml
- name: CheckoutForm
  selector: '.checkout-form'
  children:
    - name: SubmitButton
      selector: 'button[type="submit"]'
    - name: ErrorBanner
      selector: '.error-banner'
```

A tool consuming this sightmap (an event-attribution pipeline, an analytics dashboard, an
agent triaging a session) can now say `click [SubmitButton]` or `network-error
[ErrorBanner]`. What it cannot say is "this is one of the handful of things in this app that
actually matters if it breaks" — there is no field for that judgment. Every consumer either
invents its own out-of-band classification (a separate config file mapping names to
severities, drifting from the sightmap the moment a component is renamed or added) or gives
up and falls back to generic, structural signals (HTTP status codes, DOM event kind) that
don't know anything about the app's semantics.

This matters at volume. In a real session-review pipeline, selecting on structural kind
alone (e.g. "show me every network request") floods an agent or a human reviewer with noise
— the overwhelming majority of requests are analytics beacons, polling, and healthy 2xx
traffic. An author who can say "the checkout submission endpoint and this error banner are
`defect`-worthy, nothing else is" turns that flood into the handful of events that actually
warrant attention — without maintaining a second, parallel classification system that the
sightmap itself doesn't know about.

The natural authoring pattern compounds the need for tags to behave differently from names.
Authors name specific, often generic leaf elements (a button, a banner, an input) because
that's what a person actually clicks or a request actually hits. But the *judgment* — "this
area of the app matters" — is naturally made about a broader container: the whole checkout
form, not just its submit button. If tags followed the same nearest-wins rule as names, an
author tagging `CheckoutForm` as `defect` would get nothing: every real interaction happens
on a more specific, untagged child, and nearest-wins would let that child's absence of a tag
shadow the parent's presence of one. The feature would work in the trivial case (tagging the
exact leaf a user clicks) and silently fail in the common case (tagging the container that
gives that leaf meaning).

**The same shadowing risk exists wherever a sightmap picks a single winner:**

- **Views** resolve identity by most-specific-route-wins (`/checkout/payment` beats
  `/checkout/**`). Tagging the broad `/checkout/**` view `defect` needs that judgment to
  survive on URLs where a narrower, untagged view wins view identity.
- **Requests** have no winner to begin with — every matching definition already applies.
  A request's `tags` are just another field on the one definition that was always going to
  apply; no new resolution logic, included for completeness.

Scoping this SEP to `Component` alone would be an arbitrary line: the identical authoring
pattern — classify the container, not just the thing that wins identity — recurs for views
for the same structural reason. One decision, applied everywhere the spec already has an
identity-resolution rule to bypass.

## Proposal

### Shape

```yaml
# Before — a component tree with no classification
version: 1
components:
  - name: CheckoutForm
    selector: '.checkout-form'
    source: src/components/CheckoutForm.tsx
    children:
      - name: SubmitButton
        selector: 'button[type="submit"]'
      - name: ErrorBanner
        selector: '.error-banner'

# After — CheckoutForm is tagged; its children are not, and don't need to be
version: 1
components:
  - name: CheckoutForm
    selector: '.checkout-form'
    source: src/components/CheckoutForm.tsx
    tags: [defect]
    children:
      - name: SubmitButton
        selector: 'button[type="submit"]'
      - name: ErrorBanner
        selector: '.error-banner'
```

A click on `SubmitButton` or an error surfaced on `ErrorBanner` both resolve to `name:
SubmitButton` / `name: ErrorBanner` (nearest-enclosing, unchanged) **and** `tags: [defect]`
(inherited from the tagged ancestor `CheckoutForm`, via the union rule below) — even though
neither child declares a tag of its own.

A component may declare more than one tag, and multiple ancestor levels may each contribute:

```yaml
- name: CheckoutForm
  selector: '.checkout-form'
  tags: [defect]
  children:
    - name: ErrorBanner
      selector: '.error-banner'
      tags: [ui-risk]
```

A node matching `ErrorBanner` resolves `tags: [defect, ui-risk]` — the union of every
matched level, deduplicated and returned in a stable order (see Semantics).

The same field, same semantics, on a view:

```yaml
views:
  - name: Checkout
    route: /checkout/**
    tags: [defect]
  - name: CheckoutPayment
    route: /checkout/payment   # more specific: wins view IDENTITY for this URL
```

The URL `/checkout/payment` resolves to view identity `CheckoutPayment` (most-specific-route
wins, unchanged) but still resolves `tags: [defect]`, inherited from the broader `Checkout`
view — exactly the same shadowing concern as the component example, applied to route
specificity instead of DOM ancestry.

And on a request, where there is no shadowing concern to begin with:

```yaml
requests:
  - name: SubmitPayment
    route: /api/checkout/payment
    method: POST
    tags: [defect]
```

Every request whose route (and optional method) matches already applies in full — `tags`
here is not resolving anything new, it is simply another field on the one definition that
was always going to apply.

### Field reference

| Field | Type | Required | Applies to | Description |
|---|---|---|---|---|
| `tags` | string[] | no | Component, View, Request | Open-vocabulary classification labels. Not surfaced as an identity; see Semantics for how these compose (differently, per entity type). |

### JSON Schema diff

Three sibling `$defs` objects — `component`, `view`, `request` — each gain the identical new
optional property:

```
$defs.component.properties.tags:
$defs.view.properties.tags:
$defs.request.properties.tags:
  type: array
  items: { type: string, minLength: 1 }
  description: >
    Open-vocabulary classification labels applied to matches of this <entity>,
    e.g. "defect". Resolved as a union across every applicable definition — see
    SEP-0004 for the per-entity resolution rule (this is NOT the same rule used
    to resolve <entity> identity).
```

(The description differs slightly per entity to name the specific identity-resolution rule
tags bypass — see the schema file for the exact text of each.)

No new `$defs` entry is required for any of the three — unlike SEP-0003's
`componentProperty`, a tag is a bare string with no further structure. `children` already
recurses through `$defs.componentOrRef`, so a nested component gains the ability to declare
`tags:` with no further schema change, exactly as SEP-0003 noted for `properties`.

### Semantics

**The general principle.** Every entity type in scope already has a rule for resolving
*identity* — which single definition (or, for requests, which set of definitions) actually
applies to a given match. Tags never follow that rule. Instead, the effective tag set for a
match is the union of every definition's `tags` that is *applicable* to that match, whether
or not that definition wins identity. This is one principle, instantiated three times below.

**Component.** Given a matched DOM node's ancestor chain (root → leaf, the same chain the
matching algorithm already walks to resolve a name), there are two independent things to
compute over it:

- **Name** (existing, unchanged): nearest-enclosing wins. The walk from the target toward
  the root stops at the first level where any definition matches, and returns the name(s) at
  that level only. Farther ancestors are not consulted once a nearer match is found.
- **Tags** (new, this SEP): union across every matching level, no stopping. Every level of
  the walk — not just the nearest — is checked for a matching definition, and every tag any
  matching definition at any level declares is included in the result.

A leaf can simultaneously be "the submit button" (its name, resolved nearest-wins) and "part
of the defect-worthy checkout area" (its tags, resolved by union) — a container-level
judgment must not be shadowed by a leaf that happens to be named more specifically.

**View.** Given a URL, view *identity* resolves by most-specific-route-wins (see
[Route matching](../v1/schema.md#view-matching-most-specific-wins)): exactly one view is
"the current view." Tags do not follow this rule — the effective tag set for a URL is the
union of `tags` from **every** view whose route matches that URL, not just the one that wins
identity. A broad `/checkout/**` tagged `defect` still contributes that tag on
`/checkout/payment`, even though a narrower, untagged `/checkout/payment` view is what
actually supplies the view identity for that URL.

**Request.** No identity-resolution rule to bypass — "all matches apply" was already the
rule before this SEP. A matching request's `tags` are included for the same reason its
`name` already is: no new logic needed.

**Multiple contributing definitions.** Whether several component definitions match at
different ancestor levels (or the same level — analogous to name's existing "multiple
matches at that level" behavior) or several views' routes match the same URL, all of their
`tags` contribute to one deduplicated union.

**Ordering.** The resolved tag set MUST be deduplicated, for all three entity types.
Conforming SDKs SHOULD return it in a stable, lexicographically sorted order wherever it is
serialized for display, logging, or wire transmission, so two runs over the same input
produce byte-identical output and consumers can diff results deterministically. Internal
in-memory representation is not constrained beyond determinism.

**Omission.** A definition that declares no `tags` field contributes nothing; this is not an
error. `tags: []` and an absent `tags` field are equivalent — both contribute nothing. This
mirrors SEP-0003's "value omission is silent, not an error" rule.

**Interaction with `$ref` (SEP-0002).** A `$ref`-expanded component's `tags` are carried
into the deep copy exactly like every other field. No override mechanism is proposed — the
entire definition, tags included, is deep-copied, consistent with SEP-0002's existing
"Property-level override... Deferred" position and SEP-0003's identical choice for
`properties`. (`$ref` is component-only in the current spec; it does not apply to views or
requests, so this interaction is specific to the component case.)

**Interaction with `properties` (SEP-0003).** Fully orthogonal. A component may declare
both `properties` and `tags`; extraction and tag resolution do not interact.

**Interaction with `stability`.** A component or view marked with a `stability` marker may
still declare `tags`; this SEP does not couple the two. Whether a consumer should discount
tags contributed by an unstable or uncertain definition is a consumer-side policy question,
not a spec-level rule.

**Interaction with global vs. view-scoped composition.** A view's effective component and
request lists are already additive (global plus view-scoped, per
[Global vs view-scoped](../v1/schema.md#global-vs-view-scoped)). Tags need no special rule
here either: a view-scoped component's or request's `tags` are simply that definition's own
`tags`, folded into the same union as everything else applicable to the match. There is no
separate "view-scoped tags override global tags" behavior — composition is additive, same as
identity.

### Conformance

- SDKs MUST accept an optional `tags: string[]` field on any component definition (at file
  root, within a view, or under `children:`, at any nesting depth), any view definition, and
  any request definition (global or view-scoped).
- SDKs MUST resolve a matched component's effective tag set as the union of every
  definition's `tags` that matches anywhere along the node's ancestor chain — not restricted
  to the nearest-matching level used for naming.
- SDKs MUST resolve a URL's effective view-tag set as the union of every view definition's
  `tags` whose route matches that URL — not restricted to the single most-specific view that
  wins view identity.
- SDKs MUST include a matching request definition's `tags` in its output; since all matching
  requests already apply, no additional union logic beyond the existing "all matches apply"
  rule is required.
- SDKs MUST deduplicate the resolved tag set, for all three entity types.
- SDKs MUST treat a definition with no `tags` field, and one with `tags: []`, identically:
  contributing no tags, not an error.
- SDKs MUST carry `tags` through `$ref` expansion as part of the deep copy (component only;
  `$ref` does not apply to views or requests); no override mechanism exists.
- SDKs SHOULD emit the resolved tag set in a stable (lexicographically sorted) order
  wherever it is serialized for display or over the wire.
- SDKs MAY expose which specific definition(s) contributed each tag, for authoring or
  debugging tools, but MUST NOT require this for conformance.
- SDKs MUST NOT infer or assign a tag except from an explicit `tags:` entry in the sightmap
  — this field is purely author-declared; it has no relationship to any consumer's own
  intrinsic, field-derived classification (e.g. an HTTP status code, a console log level).
  Reconciling authored tags with a consumer's own classification, if any, is entirely the
  consumer's concern and out of scope for this SEP.

## Alternatives considered

### 1. Identity-resolution-only (the same rule each entity already uses)

Resolve `tags` with whichever identity rule the entity already has: nearest-enclosing-wins
for components, most-specific-route-wins for views. (Requests have no such rule to reuse —
this alternative is moot there.)

Ruled out: this defeats the motivating use case for both entity types it would apply to
(see Motivation). The dominant authoring pattern names or identifies specific, often generic
things — a leaf element, the most specific matching route — while a classification judgment
is naturally made about something broader. Identity-resolution-only would mean a tag never
applies whenever any untagged, more specific definition also matches — nearly always true in
real corpora. Consistency with the identity-resolution rule isn't worth a feature that
doesn't work for its primary case, twice over.

### 2. A different field name: `classification` or `labels`

Considered `classification: string[]` (more explicit about intent, avoids any confusion with
UI "tag chip" affordances) and `labels: string[]` (matches the Kubernetes/GitHub convention
for open-vocabulary string sets attached to an entity).

Ruled out in favor of `tags`, decided: `tags` is shorter, and it's the term already used by
the motivating reference implementation and the pipeline consuming it, so keeping the name
avoids a rename before any real corpus exists using it. `labels` was the strongest runner-up
on prior-art grounds but doesn't warrant displacing a name already proven in production.

### 3. Structured tags (`{name, value}` pairs) instead of plain strings

Modeled on SEP-0003's `properties[]`: allow a tag to carry a value, e.g. `tags: [{name:
severity, value: high}]`, rather than a bare string.

Ruled out for v1: no motivating use case demonstrates a need for tag *values* today — the
reference implementation and its consumers are boolean-membership classifications ("is this
a defect, yes or no"), not key-value pairs. Adding value support now would be speculative
complexity ahead of real demand, and — unlike `properties`, which inherently needs an
extraction directive to produce its value — a tag has no natural "how do I get the value"
question to answer. If a genuine need for valued tags emerges, it composes cleanly as a
later, additive SEP without disturbing plain-string tags: a bare string is the natural
degenerate case of `{name}` with no value.

### 4. A new top-level `tags:` concept, mapping selectors/routes to tags directly

Rather than a field on each entity, introduce a top-level section (a peer to `views:`,
`components:`, `requests:`) that maps selectors, component names, or routes to tag lists
independently of the entity definitions themselves — e.g. so tags could be authored without
touching existing entries at all.

Ruled out, more decisively with three entity types than it would be with one: this
duplicates the matching machinery `components:`, `views:`, and `requests:` already provide
— selector matching with descendant scoping, route-glob matching with specificity scoring —
purely to attach a label to the same matches a fourth, independent system would have to
rediscover. No expressive gain over "a tag is just another field on the thing that already
knows how to match."

### 5. Component-only, deferring views and requests to follow-on SEPs

Ship `tags` on `Component` alone now, and propose `View.tags`/`Request.tags` later, once
there's demonstrated demand for each.

Ruled out: the view case has the identical shadowing risk motivating this SEP in the first
place (see Motivation) — deferring it ships a known gap and asks the ecosystem to
rediscover the same problem later. The request case costs nothing to include (no new
resolution logic, just a field), so excluding it would be arbitrary, not conservative.

## Migration

Purely additive; no corpus migration is required.

- Existing `.sightmap/` directories are valid without change.
- No existing field is renamed, removed, or retyped.
- `children` already recurses through `$defs.componentOrRef`; no schema change is needed
  there to permit nested `tags:`.

**The additive-field paradox (per `spec/VERSIONING.md`).** `additionalProperties: false`
means a document using a newly-added optional field is schema-invalid under older SDKs.
Exactly like every prior additive field (`dependencies` in SEP-0001, `properties` in
SEP-0003), adopters MUST pin to the SDK version that ships this SEP before authoring `tags:`
in their own corpus; earlier SDKs reject the config with a `must NOT have additional
properties` schema error, not a silent ignore.

**Coordinated release.**

| Component | Action | Gated on |
|---|---|---|
| `sightmap/spec` | This SEP merges; schema published | — |
| `sightmap-go` | Re-vendor schema; implement `tags` resolution for components (union across ancestor levels), views (union across matching routes), and requests (already-applying) | spec merge |
| `sightmap-js` | Re-vendor schema; implement identical resolution semantics, independently verified against the shared conformance fixture | spec merge |
| Docs (`sightmap.org` authoring guide) | Document `tags:` alongside `properties:`/`memory:` | at least one SDK landing |
| Adopters | Pin to the new SDK version(s) before authoring `tags:` in any corpus | relevant SDK release |

`sightmap-go` and `sightmap-js` implementations are independent of each other, both gated
only on this SEP merging. Docs and adopter rollout should follow at least one real
implementation landing, so the authoring guide can link a working release rather than a
merged-but-unimplemented spec change.

## Open questions

1. **Structured/valued tags.** Should a future SEP revisit `{name, value}` tags once real
   demand exists? See Alternative 3. Deliberately left unaddressed here.
2. **Console/exception classification.** Console and exception events aren't a matchable
   entity in the spec today (no selector or route to attach a definition to). Tags there
   would need a different mechanism than this SEP proposes — left to a follow-on SEP if a
   real use case emerges.
3. **Canonical formatting.** `dependencies[]` (SEP-0001) is canonicalized — sorted and
   deduplicated — by the `fmt` command, with a dedicated conformance fixture
   (`108-fmt-dependencies-canonical`). Should `tags[]`, being similarly an unordered set
   rather than an ordered list (unlike `properties[]`, where authoring order is meaningful),
   get the same treatment? This SEP proposes runtime resolution be deduplicated and stably
   ordered (see Semantics) but does not commit to a specific canonical-format fixture number
   or a MUST-level authoring-time formatting rule — left for reviewers to decide.
4. **Tag namespacing.** Should there be any reserved-prefix or namespacing convention (as
   `stability:` has a closed enum of reserved values) to avoid two authors' tags colliding in
   meaning across a large, multi-team corpus? No convention is proposed in v1; tags are a
   flat, open vocabulary, consistent with how `name` itself has no namespacing today.

## References

- [SEP-0001](0001-dependencies-field.md) (`dependencies[]`) — the closest precedent for an
  optional, additive, array-of-strings field with a coordinated-release migration story.
- [SEP-0002](0002-component-ref.md) (`$ref`) — the deep-copy mechanism `tags` composes with
  unchanged.
- [SEP-0003](0003-component-properties.md) (`properties[]`) — the closest precedent for an
  additive component-level array field with its own resolution semantics, and the model this
  SEP's structure follows most closely.
- [`spec/v1/sightmap.schema.json`](../v1/sightmap.schema.json) — the schema update this SEP
  proposes.
- [`spec/v1/schema.md`](../v1/schema.md) — the human-readable counterpart: field tables for
  `Component`/`View`/`Request`, and the dedicated `## Tags` section.
- [`spec/VERSIONING.md`](../VERSIONING.md) — the additive-field/coordinated-release policy
  this SEP's Migration section follows.
- Conformance fixture `017-tags.fixture/` — schema-shape validation across all three entity
  types, verified against the schema change in this SEP (`npm run validate:conformance`: 29
  files checked, 0 failures) alongside the full existing conformance and example suites. The
  offline, cross-language conformance suite has no live DOM or URL router to exercise, so it
  cannot test the union resolution rule itself (the same limitation SEP-0003's `properties`
  extraction ran into); that behavior is asserted by each SDK's own test suite instead.
- Reference implementation: Fullstory's internal Subtext session-review signal pipeline (Go)
  implements and unit-tests the component case's union-across-levels resolution today (the
  view and request cases are new in this SEP, not yet implemented anywhere). Not a public
  repository — cited here for provenance, not as a checkable link.
