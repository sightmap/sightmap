---
sep: 0009
title: Component capture privacy via `privacy`
author: Clint Ayres (@jurassix)
status: Review
created: 2026-09-29
updated: 2026-09-29
spec-version-target: 1
related-issues: []
related-discussions: []
---

## Summary

Add an optional `privacy: block | mask | unmask` field to `Component` entries. It declares
whether a capture consumer, meaning anything that records the page for later replay or
analysis, may retain the matched element's content. `block` withholds the element entirely,
`mask` retains its shape but not its text, and `unmask` retains it in full, overriding a
broader `block` or `mask` that would otherwise cover it. The directive applies to the matched
element and its subtree, and the nearest enclosing declaration wins, so the common shape is a
`mask` on a form with an `unmask` on the one field inside it that is safe to keep. It governs
[extracted properties](0010-tree-closed-component-properties.md) as well as captured content, at
the node each value is read from rather than at the component that declared it, and it draws an
explicit line through attributes, which can be either structure or content.

## Motivation

A sightmap already names the parts of an app that matter. The same corpus is consumed by tools
that record those parts, and every one of them has to be told, separately and in its own
configuration language, which of them must not be recorded. That configuration is maintained by
hand, keyed on CSS selectors, in a system that has no idea the selectors correspond to named
components. The two drift, and the direction they drift in is the dangerous one: a component is
renamed or its selector is retuned in the corpus, the privacy rule keyed on the old selector
silently stops matching, and content that was meant to be withheld is captured instead.

Three things follow from the split that a corpus-side field fixes:

- **The knowledge is already in the corpus, in the wrong half.** A curator writing
  `name: SocialSecurityInput` knows in that moment that it must never be recorded. Nothing in the
  spec lets them say so, so the fact is re-derived later by someone reading a selector.
- **Privacy cannot be reviewed alongside what it protects.** A reviewer looking at a corpus diff
  that adds a payment form cannot see whether the card field is covered. The answer lives in
  another system, behind different access control, with its own change history.
- **A silent failure mode.** Selector-keyed rules fail open. When the selector stops matching,
  nothing errors: the element is simply captured. A corpus-side declaration fails the other way,
  because a component whose selector stops matching is already a drift signal the corpus reports.

This is not a hypothetical shape: consumers already carry per-component privacy in side files
keyed by component name, because the schema gives them nowhere else to put it.

Every mainstream session-capture tool has this three-way concept under its own names, and each
applies it by CSS selector. Naming it once, on the component, is less work than each of them
naming it separately against selectors they do not own.

## Proposal

### Shape

A `Component` entry gains an optional `privacy` field taking one of three values.

```yaml
# Before: the corpus names the fields but says nothing about capturing them
components:
  - name: CheckoutForm
    selector: '.checkout-form'
    children:
      - name: CardNumberInput
        selector: 'input[name="cc"]'
      - name: OrderId
        selector: '.confirmation-order-id'

# After
components:
  - name: CheckoutForm
    selector: '.checkout-form'
    privacy: mask            # retain the form's shape, not its content
    children:
      - name: CardNumberInput
        selector: 'input[name="cc"]'
        privacy: block       # withhold entirely, not even shape
      - name: OrderTotal
        selector: '.order-total'
        privacy: unmask      # safe to keep, despite the mask above
```

| Value | Meaning |
|---|---|
| `block` | The element and its subtree MUST NOT be captured. Neither content nor structure is retained. |
| `mask` | The element's structure and layout MAY be captured; its text, input values, and attribute values MUST NOT be, except the interactive-state attributes named under [Attributes](#attributes-under-mask). |
| `unmask` | The element and its subtree are captured in full, overriding any enclosing `block` or `mask`. |

### JSON Schema

- `$defs.component`: add an optional `privacy` property,
  `{ type: string, enum: [block, mask, unmask] }`. No change to `required`; `$defs.component`
  keeps `additionalProperties: false`.
- The Go SDK's `componentFields` unknown-field allowlist gains `"privacy"`.
- No other `$defs` entry changes, and no existing property changes type or requiredness.

### Semantics

**Scope.** A declaration applies to the matched element and every descendant of it, not only to
the element itself. Masking a form masks the fields inside it without naming each one.

**Resolution is nearest-enclosing wins**, the same rule component *identity* already follows.
Where several declarations cover one element, the one on the innermost matching component decides,
and it decides for that component's whole subtree until another declaration overrides it again.
This is what makes `unmask` meaningful: it has no effect in isolation, and exists so an author can
carve one safe element out of a broader restriction.

Note this deliberately differs from `tags` ([SEP-0004](0004-component-tags.md)), which resolve as a
union across every applicable definition. A union is right for classification, where more labels
are additive and harmless. It is wrong for a directive, where two applicable values are a
contradiction that has to be decided rather than merged.

**Omission declares nothing.** A component with no `privacy` field makes no statement. It does not
mean "capture this": it means this corpus is silent, and whatever default the consumer already
applies is unchanged. A corpus can therefore adopt the field incrementally, one component at a
time, without implying anything about the components it has not reached.

**The corpus is a floor, not a ceiling.** A consumer MAY withhold more than the corpus asks, for
instance because of its own configuration or a regulatory default. A consumer MUST NOT capture
something the corpus marked `block` or `mask`. An `unmask` is a statement by the corpus author that
the element is safe, not an instruction that overrides the consumer's own policy; a consumer that
blocks the element for its own reasons MUST continue to.

**Extracted properties are governed too, at the node the value is read from.** A `properties[]`
entry ([SEP-0010](0010-tree-closed-component-properties.md)) produces a named value that travels
separately from captured content, so a directive covering only the recording would leak the same
text through the other path. A property whose value resolves from a node whose effective privacy
is `block` or `mask` MUST NOT be surfaced.

The node that matters is the one the value is **read from**, not the component that declared the
property. SEP-0010 extraction is tree-closed, so a `PATH.prop` directive resolves against a
descendant component, and that descendant's own resolved privacy governs. A component with no
declaration of its own therefore cannot launder a value out of a blocked descendant:

```yaml
- name: CheckoutSummary
  selector: '.summary'
  properties:
    - name: card                      # declared on an unrestricted component...
      extract: CardNumberInput.value  # ...but read from a blocked one, so it is withheld
  children:
    - name: CardNumberInput
      selector: 'input[name="cc"]'
      privacy: block
      properties:
        - name: value
          extract: attr=value
```

`exists:PATH` is the one exception, because it reports presence rather than content. Under `mask`
it MAY be surfaced, since `mask` already permits structure. Under `block` it MUST NOT be, since a
blocked element is not reported at all. Every other extract form (`text`, `raw_text`, `attr=NAME`,
`PATH.prop`) yields content and is withheld under either.

`attr=` is withheld under `mask` for every attribute **except** the four interactive-state names
below, which resolve normally. Under `block` nothing resolves, state included, because a blocked
element is not reported at all.

#### Attributes under `mask`

`mask` permits structure and withholds content, and an attribute can be either, so the line has to
be drawn in the spec rather than left to each consumer. Leaving it open is the gap this closes: a
`data-email` attribute is neither text nor an input value, so a consumer reading `mask` narrowly
could retain it in the recording while the corpus believed the element was protected.

A consumer MUST withhold, under `mask`:

- **The value of any attribute the corpus reads via an `attr=` extract, anywhere in the corpus.** An
  author who names an attribute in an extract has declared that it carries a value worth reading,
  which is the definition of content. Tying the rule to the corpus rather than to a fixed list is
  what keeps it from going stale.
- **The value of any `data-*` attribute**, and of `value`, `title`, `alt`, `placeholder`, and
  `aria-label`. These routinely carry user-visible or user-derived text.

A consumer MAY retain, under `mask`:

- **The four interactive-state attributes [SEP-0013](0013-richer-node-data.md) defines** —
  `checked`, `selected`, `disabled`, `expanded`. This is a closed set fixed by the spec rather than
  an open allowlist, and each carries a state rather than a value. Withholding them would make
  `mask` and SEP-0013 mutually unusable, since a masked form could then report no control state at
  all, which is most of what makes a masked form worth recording.
- **Presentational attributes needed to render the element's shape**, such as `class`, `style` and
  `id`.

That last permission is a residual risk, and it is stated rather than hidden: a `class` or `id`
built from user data, `class="user-jane-doe"`, survives a `mask`. The spec cannot close it without
making `mask` unimplementable for replay, which needs those attributes to render anything at all.
An author carrying user data in a presentational attribute should reach for `block`.

Withholding follows the existing omission rule: the property is **absent**, exactly as if it had
not resolved. SEP-0010 already requires consumers to treat omission as normal rather than as an
error, so this introduces no new failure mode, and a consumer cannot distinguish "withheld" from
"did not resolve". That is deliberate; the distinction would itself disclose something about the
element.

**This is a capture-time directive, not a matching input.** `privacy` does not participate in route
matching, component identity, or specificity. Two components differing only in `privacy` are the
same component for every other purpose.

### Conformance

A conforming capture consumer MUST:

- Accept `privacy` as an optional `Component` property with the three enumerated values, at every
  depth including recursive `children`.
- Apply a declaration to the matched element and its entire subtree.
- Resolve competing declarations by nearest-enclosing, so an inner declaration overrides an outer
  one for the inner component's subtree.
- Treat an absent `privacy` as declaring nothing, and leave its own default behavior unchanged.
- Never capture content the resolved value marks `block` or `mask`, regardless of its own defaults.
- Withhold any `properties[]` value whose node resolves to `block` or `mask`, judged at the node
  the value is read from rather than at the declaring component, and omit it the way an unresolved
  property is omitted. Under `mask`, `exists:PATH` and the four interactive-state attributes MAY
  still resolve; under `block` nothing resolves.
- Under `mask`, withhold from the captured recording the value of every `data-*` attribute, of
  `value`/`title`/`alt`/`placeholder`/`aria-label`, and of any attribute named by an `attr=` extract
  anywhere in the corpus.

A conforming consumer that does not capture page content at all, such as an offline matcher or a
documentation generator, MUST accept and ignore the field.

A conforming SDK MUST pass the shared conformance fixture `spec/conformance/021-component-privacy.fixture/`.

## Alternatives considered

1. **Leave it in each consumer's own configuration.** The status quo. Rejected because it is the
   thing that produces the silent failure described in Motivation: two systems keyed on the same
   selectors, only one of which knows when a selector changes.

2. **Put it on `properties[]` instead of the component.** A property already names a value extracted
   from an element, so a `privacy` flag there looks natural. Rejected because it covers the wrong
   thing. Capture privacy is about the element's content in the recording, not about a value a
   consumer pulled out of it, and most elements that need blocking have no declared property at all.

3. **Union resolution, matching `tags`.** Rejected: see Semantics. Merging `block` and `unmask` has
   no sensible result, and picking one silently is worse than picking one by a stated rule.

4. **A boolean `private: true` instead of a three-value enum.** Simpler, and it covers the common
   case. Rejected on two counts: it cannot express the mask-the-form-unmask-one-field shape that is
   the reason authors reach for this at all, and every consumer surveyed already distinguishes
   "withhold entirely" from "keep the shape", so a boolean would have to be mapped onto one of them
   arbitrarily.

5. **Extend it to views and requests in this SEP.** A view-level default and a request-level body
   restriction are both real needs. Deferred deliberately, per the SEP process's one-decision rule.
   Component privacy stands alone and does not foreclose either; a later SEP can add them with the
   resolution rule this one establishes.

## Migration

No corpus migration is required. The field is optional and additive, and a corpus that omits it
declares nothing, which is exactly its behavior today.

**The additive-field pin applies**, as it does to every optional field added under
`additionalProperties: false` (see `spec/VERSIONING.md`, and the same note in
[SEP-0004](0004-component-tags.md)). An SDK older than the one shipping this SEP rejects a corpus
using `privacy` with a `must NOT have additional properties` schema error rather than ignoring it.
Adopters pin to the SDK version that ships it before authoring the field.

**Consumers that already hold selector-keyed privacy configuration** should treat corpus
declarations as additive on first adoption rather than as a replacement, and reconcile the two
deliberately. Silently dropping existing rules in favour of a partially-annotated corpus is the one
migration path that can expose content, and no consumer should take it automatically.

## Open questions

1. **Should `unmask` be constrained to appear only beneath a `block` or `mask`?** It is inert
   anywhere else, so an unconstrained `unmask` on a component with no enclosing restriction is
   almost certainly an authoring error worth a diagnostic. Proposed as a warning rather than a
   schema constraint, since the enclosing declaration can live in another file.

2. **Should a consumer be required to report coverage?** A corpus author has no way to ask "which
   of my components are captured in full". A `sightmap` subcommand reporting resolved privacy per
   component would make the field reviewable, but it is tooling rather than spec and is left out of
   this proposal.

3. **Does `mask` need to say what replaces the content?** Consumers differ: some substitute
   same-length placeholder characters, some a fixed string, some drop the text node. Leaving it
   implementation-defined keeps the field portable, at the cost of a corpus author not knowing
   exactly what a replay will show.

## References

- [SEP-0004](0004-component-tags.md): the closest precedent for an optional authored field on
  `Component`, and the source of the resolution rule this SEP deliberately departs from.
- [`spec/v1/schema.md#component`](../v1/schema.md#component): the entry this SEP extends.
- [`spec/VERSIONING.md`](../VERSIONING.md): the additive-field pin requirement.

