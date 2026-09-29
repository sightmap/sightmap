---
sep: 0015
title: Component visibility reporting via `watch`
author: Clint Ayres (@jurassix)
status: Draft
created: 2026-09-29
updated: 2026-09-29
spec-version-target: 1
related-issues: []
related-discussions: []
---

## Summary

Add an optional `watch: true` field to `Component` entries. It asks a capture consumer to report
this component's **visibility lifecycle** — that it rendered, that it came into view, that it went
away — rather than only reporting it when someone interacts with it. A watched component is
reported whether or not a user ever touches it, which is the point: the interesting fact about a
promotional banner, an error state, or an empty-results message is usually that it appeared at all.

## Motivation

A sightmap's components are named because they matter. But the runtime record a consumer produces
is overwhelmingly **interaction-shaped**: it says what was clicked, typed into, or selected.
Everything a user merely *saw* is either absent or has to be recovered by replaying the session and
looking.

That gap is awkward for exactly the components authors care most about, because the ones whose
appearance is the signal are usually the ones nobody clicks:

- **An error or empty state.** `NoResultsMessage` rendering is the whole event. It is never clicked,
  so an interaction-shaped record contains no trace of it, and "how often do users hit an empty
  search" cannot be answered from the data at all.
- **Content below the fold.** A page can contain a section that most sessions never scroll to.
  Whether it was *seen* and whether it was *present* are different questions, and only the second
  is currently answerable.
- **Anything conditional.** A promotion, a variant, an upsell, a beta banner. Its presence is a fact
  about that session, and it is invisible unless it happens to be interactive.

The workaround today is to instrument each of these by hand, in the consumer's own configuration,
keyed on a CSS selector the corpus already knows. That is the same duplication SEP-0009 describes
for privacy, with the same failure mode: the selector drifts, the hand-written rule silently stops
matching, and the signal disappears with no error.

There is a second, subtler reason the corpus is the right place. A consumer that reports
interactions has a good reason to ignore non-interactive elements, and many do so by design. That
heuristic is correct for its own purpose and precisely wrong here, because the components worth
watching are the non-interactive ones. Only the corpus author knows which ones those are, and today
they have no way to say so.

## Proposal

### Shape

A `Component` entry gains an optional boolean `watch`.

```yaml
# Before: the corpus names them, but only the button is ever reported
components:
  - name: NoResultsMessage
    selector: '.search-empty'
  - name: PromoBanner
    selector: '[data-promo]'
  - name: RetrySearchButton
    selector: 'button.retry'

# After
components:
  - name: NoResultsMessage
    selector: '.search-empty'
    watch: true
  - name: PromoBanner
    selector: '[data-promo]'
    watch: true
  - name: RetrySearchButton
    selector: 'button.retry'   # already reported when clicked; no watch needed
```

### JSON Schema

- `$defs.component`: add an optional `watch` property, `{ type: boolean }`. No change to
  `required`; `$defs.component` keeps `additionalProperties: false`.
- The Go SDK's `componentFields` unknown-field allowlist gains `"watch"`.
- No other `$defs` entry changes, and no existing property changes type or requiredness.

### Semantics

**`watch: true` asks for visibility reporting.** A consumer that reports visibility SHOULD report,
for each matched element: that it was rendered into the page, that it became visible to the user,
and that it stopped being present or visible. The three are a lifecycle, not an enumeration — a
consumer reports them under whatever names and granularity it already uses, and a consumer with
only a coarser notion of "seen" satisfies this with that.

**`watch` does not apply to the subtree.** It marks the component it is written on, and nothing
else. This is the opposite of [SEP-0009](0009-component-privacy.md) `privacy`, and the difference is
deliberate: privacy is a restriction, where covering the subtree is the safe default, while `watch`
generates records, where covering a subtree silently would multiply them. Watching children means
marking the children.

**Each matched element is reported separately.** A component matching several elements on a page
yields one lifecycle per element, not one for the component. A consumer that cannot distinguish
instances MUST still report at least the first.

**`watch: false` is the same as omitting it.** The field exists to opt in. It is permitted as an
explicit value so a generated corpus can round-trip, but it declares nothing.

**A consumer's interactivity heuristic MUST NOT suppress a watched component.** A consumer that
would otherwise skip an element because it is not interactive MUST report a watched one anyway.
This is the rule that makes the field worth having: without it, the field silently does nothing for
exactly the non-interactive components that motivate it. Other filters a consumer applies for its
own correctness (a selector its engine cannot express, a hard cap on watched elements) are
unaffected, but such a consumer SHOULD surface that it dropped the request rather than ignoring it.

**`watch` takes no part in matching.** It does not affect route matching, component identity, or
specificity. Two components differing only in `watch` are the same component for every other
purpose.

**Interaction with `privacy`.** The two are independent axes and compose without special rules. A
component may be both watched and blocked: the consumer reports that it appeared while retaining
none of its content. That combination is useful rather than contradictory — "the user saw the SSN
field" is a fact worth having without the value.

### Conformance

A conforming consumer that reports visibility MUST:

- Accept `watch` as an optional boolean `Component` property, at every depth including recursive
  `children`.
- Report the visibility lifecycle of each element matched by a component with `watch: true`.
- Report a watched component regardless of whether it is interactive, and regardless of whether any
  interaction with it ever occurs.
- Treat `watch: false` and an absent `watch` identically.
- Apply `watch` to the component it is declared on only, never to its `children`.

A conforming consumer that does not report visibility at all, such as an offline matcher or a
documentation generator, MUST accept and ignore the field.

A conforming SDK MUST pass the shared conformance fixture `spec/conformance/022-component-watch.fixture/`.

## Alternatives considered

1. **Leave it in each consumer's configuration.** The status quo, and the same argument as
   SEP-0009: a hand-maintained rule keyed on a selector the corpus owns, which fails silently when
   the selector moves.

2. **Infer it, rather than declaring it.** A consumer could watch every component in the corpus, or
   every non-interactive one. Rejected: a corpus routinely names hundreds of components, most of
   which appear on every page render, and reporting all of them produces volume without signal. The
   author's judgement about which appearances are interesting is exactly the information the field
   carries.

3. **An enum instead of a boolean** — `watch: rendered | visible | full`. Rejected for now. The
   lifecycle a consumer can report is a property of that consumer, not of the component, and a
   corpus author picking a granularity would be guessing at a capability they cannot see. If a real
   need to ask for less than the full lifecycle emerges, the boolean can widen to a union later
   without breaking `watch: true`.

4. **Put it on the view instead.** "Report when this view is reached" is already answerable from
   navigation, so a view-level form would duplicate it. The component is where the unanswered
   question lives.

5. **Fold it into `privacy` as one `capture:` block.** Both are capture-layer directives, so one
   nested field holding both is tempting. Rejected on the SEP process's one-decision rule, and
   because they genuinely differ in scope: privacy covers a subtree, `watch` does not. Nesting them
   would put two different resolution rules under one key.

## Migration

No corpus migration is required. The field is optional and additive, and a corpus that omits it
behaves exactly as it does today.

**The additive-field pin applies**, as for every optional field added under
`additionalProperties: false` (see `spec/VERSIONING.md`). An SDK older than the one shipping this
SEP rejects a corpus using `watch` with a `must NOT have additional properties` error rather than
ignoring it. Adopters pin to the SDK version that ships it before authoring the field.

**Adopting the field costs record volume**, which is worth saying plainly. Every watched component
produces records for every session in which it renders. A consumer that meters or bills on record
volume should expect the increase to scale with how many components an author marks, and authors
should mark the ones whose appearance is a signal rather than marking broadly.

## Open questions

1. **Should there be a cap, or a recommended ceiling?** Nothing in this proposal stops an author
   marking every component in a corpus. A MAY-level guideline, or a lint warning above some count,
   might be worth having; it is left out because the right number is consumer-specific.

2. **Should a consumer report a watched component that never matched?** "The banner did not appear"
   can be more informative than its appearance. It is also unbounded, since it requires reporting
   an absence for every watched component on every page, so this proposal does not require it.

3. **Does `watch` need to interact with `stability`?** A component marked `stability: unstable` is
   one whose selector is known to break. Watching one produces a signal that silently stops rather
   than one that reports absence, which may deserve a diagnostic at authoring time.

## References

- [SEP-0009](0009-component-privacy.md): the other capture-layer directive on `Component`, and the
  source of the scope contrast drawn in Semantics.
- [SEP-0004](0004-component-tags.md): the precedent for an optional authored field on `Component`.
- [`spec/v1/schema.md#component`](../v1/schema.md#component): the entry this SEP extends.
- [`spec/VERSIONING.md`](../VERSIONING.md): the additive-field pin requirement.

## Appendix: example consumer mapping (non-normative)

This appendix is illustrative. It is not part of the specification, and a conforming consumer is
under no obligation to resemble it.

Fullstory expresses this as an element watch, registered against a named element it already holds
an identifier for, and emits an element-seen event with three states: `RENDERED`, `VISIBLE`, and
`END`. A compiler mapping `watch: true` therefore chains onto whatever it already does for the
component itself — it creates the named element first, then registers the watch against that
element's identifier.

That ordering is the practical reason for the interactivity rule in Semantics: a consumer whose
element-creation step filters to interactive elements will never create the named element for a
non-interactive watched component, so the watch has nothing to attach to and the field silently
does nothing. The rule exists so that filter is bypassed for watched components specifically.
