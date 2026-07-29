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

Add an optional `tags: string[]` field to `Component` entries. A tag is an open-vocabulary
classification label — `defect`, `ui-risk`, whatever an author needs — distinct from a
component's `name`. Where `name` answers "what is this element called," `tags` answers
"does anything that happens here belong to some cross-cutting classification I care about."
Unlike name resolution, which is nearest-enclosing-wins, tag resolution is a **union across
every matching ancestor level** — a tag authored on a broad container rides everything
inside it, not just the target element itself.

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

### Field reference

| Field | Type | Required | Description |
|---|---|---|---|
| `tags` | string[] | no | Open-vocabulary classification labels for this component. Not surfaced as a name; see Semantics for how these compose across the ancestor chain. |

### JSON Schema diff

The `$defs.component` object gains one new optional property:

```
$defs.component.properties.tags:
  type: array
  items: { type: string, minLength: 1 }
  description: >
    Open-vocabulary classification labels applied to matches of this component,
    e.g. "defect". Resolved as a union across every matching ancestor level, not
    just the nearest-enclosing match used for naming. See SEP-0004.
```

No new `$defs` entry is required — unlike SEP-0003's `componentProperty`, a tag is a bare
string with no further structure. `children` already recurses through
`$defs.componentOrRef`, so a nested component gains the ability to declare `tags:` with no
further schema change, exactly as SEP-0003 noted for `properties`.

### Semantics

**Two resolution rules for the same ancestor walk.** Given a matched DOM node's ancestor
chain (root → leaf, the same chain the matching algorithm already walks to resolve a name),
there are now two independent things to compute over it:

- **Name** (existing, unchanged): nearest-enclosing wins. The walk from the target toward
  the root stops at the first level where any definition matches, and returns the name(s) at
  that level only. Farther ancestors are not consulted once a nearer match is found.
- **Tags** (new, this SEP): union across every matching level, no stopping. Every level of
  the walk — not just the nearest — is checked for a matching definition, and every tag any
  matching definition at any level declares is included in the result.

These are deliberately different rules for the same walk, because they answer different
questions. Naming needs exactly one right answer (the most specific thing this element is).
Classification does not have that constraint — a leaf can simultaneously be "the submit
button" and "part of the defect-worthy checkout area," and a container-level judgment must
not be shadowed by a leaf that happens to be named more specifically.

**Combining multiple contributing definitions.** When several definitions match at
different levels of the same chain (as in the two-tag example above), or when several
definitions match at the *same* level (analogous to name's existing "multiple matches at
that level" behavior), all of their `tags` contribute. The effective tag set for a matched
node is the union of every matching definition's `tags`, across every level, deduplicated.

**Ordering.** The resolved tag set MUST be deduplicated. Conforming SDKs SHOULD return it in
a stable, lexicographically sorted order wherever it is serialized for display, logging, or
wire transmission, so two runs over the same session produce byte-identical output and
consumers can diff results deterministically. Internal in-memory representation is not
constrained beyond determinism.

**Omission.** A component that declares no `tags` field contributes nothing; this is not an
error. `tags: []` and an absent `tags` field are equivalent — both contribute nothing. This
mirrors SEP-0003's "value omission is silent, not an error" rule.

**Interaction with `$ref` (SEP-0002).** A `$ref`-expanded component's `tags` are carried
into the deep copy exactly like every other field. No override mechanism is proposed — the
entire definition, tags included, is deep-copied, consistent with SEP-0002's existing
"Property-level override... Deferred" position and SEP-0003's identical choice for
`properties`.

**Interaction with `properties` (SEP-0003).** Fully orthogonal. A component may declare
both `properties` and `tags`; extraction and tag resolution do not interact.

**Interaction with `stability`.** A component marked `stability: unstable` or `uncertain`
may still declare `tags`; this SEP does not couple the two. Whether a consumer should
discount tags contributed by an unstable ancestor is a consumer-side policy question, not a
spec-level rule.

### Conformance

- SDKs MUST accept an optional `tags: string[]` field on any component definition, at file
  root, within a view, or under `children:`, at any nesting depth.
- SDKs MUST resolve a matched node's effective tag set as the union of every definition's
  `tags` that matches anywhere along the node's ancestor chain — not restricted to the
  nearest-matching level used for naming.
- SDKs MUST deduplicate the resolved tag set.
- SDKs MUST treat a component with no `tags` field, and a component with `tags: []`,
  identically: contributing no tags, not an error.
- SDKs MUST carry `tags` through `$ref` expansion as part of the deep copy; no override
  mechanism exists.
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

### 1. Nearest-wins-only (the same rule as naming)

Resolve `tags` with the identical nearest-enclosing-wins rule already used for `name`, for
consistency with the existing resolution model and to avoid introducing a second kind of
walk.

Ruled out: this defeats the motivating use case outlined above. The dominant authoring
pattern names specific, often generic leaf elements while a classification judgment is
naturally made about a broader container. Nearest-wins would mean a tag never applies
whenever any untagged, more specific definition also matches the target — which is nearly
always true in real corpora, since sightmaps name leaves far more often than they tag
containers. A consistency argument is not worth shipping a feature that doesn't work for
its primary case.

### 2. A different field name: `classification` or `labels`

Considered `classification: string[]` (more explicit about intent, avoids any confusion with
UI "tag chip" affordances) and `labels: string[]` (matches the Kubernetes/GitHub convention
for open-vocabulary string sets attached to an entity).

Ruled out in favor of `tags`, but noted as the closest open question (see below):
`tags` is shorter, and it's the term already used by the motivating reference
implementation and the pipeline consuming it, so keeping the name avoids a rename before any
real corpus exists using it. `labels` was the strongest runner-up on prior-art grounds; if a
reviewer has a strong preference either way, this is a cheap, purely cosmetic change to make
before merge.

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

### 4. A new top-level `tags:` concept, mapping selectors to tags directly

Rather than a field on `Component`, introduce a top-level section (a peer to `views:`,
`components:`, `requests:`) that maps selectors or component names to tag lists
independently of component definitions — e.g. so tags could be authored without touching
existing component entries at all.

Ruled out: this duplicates the exact selector-matching machinery `components:` already
provides — a name plus one or more selectors, with the same descendant/child-scoping rules
— purely to attach a different label to the same match. It would mean two independent
matching systems doing the same DOM-resolution work in one spec, doubling what every SDK and
the shared conformance suite must implement, for no expressive gain over "a tag is just
another field on the thing that already knows how to match a DOM subtree."

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
| `sightmap-go` | Re-vendor schema; implement `tags` union-across-levels resolution in the matcher | spec merge |
| `sightmap-js` | Re-vendor schema; implement identical resolution semantics, independently verified against the shared conformance fixture | spec merge |
| Docs (`sightmap.org` authoring guide) | Document `tags:` alongside `properties:`/`memory:` | at least one SDK landing |
| Adopters | Pin to the new SDK version(s) before authoring `tags:` in any corpus | relevant SDK release |

`sightmap-go` and `sightmap-js` implementations are independent of each other, both gated
only on this SEP merging. Docs and adopter rollout should follow at least one real
implementation landing, so the authoring guide can link a working release rather than a
merged-but-unimplemented spec change.

## Open questions

1. **Field name.** `tags` vs. `labels` vs. `classification` — see Alternative 2. No strong
   objection is anticipated, but this is the cheapest thing to change before merge if a
   reviewer feels strongly.
2. **Structured/valued tags.** Should a future SEP revisit `{name, value}` tags once real
   demand exists? See Alternative 3. Deliberately left unaddressed here.
3. **Non-DOM classification (network, console).** The motivating use case eventually wants
   classification on non-DOM signals too — an API request, a console error — not just DOM
   components. This SEP scopes to `Component.tags` only. Whether `requests:` gets its own
   `tags:` field (once its route-matching semantics support the kind of ancestor-composition
   this SEP relies on) is left to a follow-on SEP; requests don't currently have an analogous
   containment hierarchy to union across.
4. **Canonical formatting.** `dependencies[]` (SEP-0001) is canonicalized — sorted and
   deduplicated — by the `fmt` command, with a dedicated conformance fixture
   (`108-fmt-dependencies-canonical`). Should `tags[]`, being similarly an unordered set
   rather than an ordered list (unlike `properties[]`, where authoring order is meaningful),
   get the same treatment? This SEP proposes runtime resolution be deduplicated and stably
   ordered (see Semantics) but does not commit to a specific canonical-format fixture number
   or a MUST-level authoring-time formatting rule — left for reviewers to decide alongside
   the field-name question.
5. **Tag namespacing.** Should there be any reserved-prefix or namespacing convention (as
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
- [`spec/v1/schema.md`](../v1/schema.md) — the human-readable counterpart, `## Component`
  section and field table.
- [`spec/VERSIONING.md`](../VERSIONING.md) — the additive-field/coordinated-release policy
  this SEP's Migration section follows.
- Conformance fixture `017-component-tags.fixture/` — schema-shape validation, verified
  against the schema change in this SEP (`npm run validate:conformance`: 29 files checked,
  0 failures) alongside the full existing conformance and example suites. The offline,
  cross-language conformance suite has no live DOM to walk, so it cannot exercise the
  union-across-levels resolution rule itself (the same limitation SEP-0003's `properties`
  extraction ran into); that behavior is asserted by each SDK's own test suite instead.
- Reference implementation: Fullstory's internal Subtext session-review signal pipeline
  (Go) implements and unit-tests this exact union-across-levels resolution rule today. Not
  a public repository — cited here for provenance, not as a checkable link.
