---
sep: 0023
title: The most specific definition names an element
author: Joel Webber (@joelgwebber)
status: Draft
created: 2026-10-22
updated: 2026-10-22
spec-version-target: 1
related-issues: []
related-discussions: []
---

## Summary

When an element is claimed by more than one component definition, the one whose selector is **strictly more specific** names it: it requires everything the other requires and more. `span.label` names a labelled span even though `span` also matches it; `[data-id^="row-"][data-id$="-total-price"]` names a total even though `[data-id$="-price"]` also matches it; a child definition names an element its parent's bare global also reaches. Specificity is decided from the selector text alone by a procedure this SEP fixes, so every SDK orders the same pairs. Definitions the procedure cannot order stay what they are today: an ambiguity for tooling to report.

## Motivation

A corpus that names both a general thing and a special case of it has to write the general one so it excludes the special one. CSS has no way to say "and no other class" or "and nothing more specific", so the only tool is `:not()`:

```yaml
- name: Text
  selector: 'span:not(.label):not(.hint):not(.price):not([data-id$="-price"])'
- name: Label
  selector: span.label
```

Every new special case edits the general definition. Generated corpora show where this ends. A corpus derived from captures of a tax-preparation application had a bare `span` kind whose selector carried thirty-five `:not()` clauses, one per sibling kind that refined it, and ran to 1,160 bytes; in a corpus of a large SaaS application, 185 of 1,979 definitions carried a `:not()` or `:has()`, nearly all to exclude a more specific neighbour. SEP-0021 (the portable selector profile) caps a selector at 1,024 bytes and bars `:not()` around anything but a simple selector, so those corpora cannot be made portable without removing the clauses, and removing them makes every labelled span contested.

The same shape appears without classes. A site's test ids are paths with instance indices (`personalinfo-summary-views-0-summary-collection-values-6-collection-values-2-action-label-text`). A generator that stems them writes `[data-automation-id^="…values-"][data-automation-id$="-text"]` for the general value and `[…^="…values-6-collection-values-"][…$="-action-label-text"]` for the action label. The second is a special case of the first, and nothing short of enumerating the first's exclusions keeps them apart. Seventy-nine elements of one such corpus were contested this way.

Authors expect the more specific definition to win. It is how the CSS cascade reads, how route matching already works (most specific wins, [View matching](../v1/schema.md#view-matching-most-specific-wins)), and what the reference matcher already does in one special case: a child definition's chain beats a bare global regardless of declaration order. This SEP states the general rule.

## Proposal

### Shape

No new fields. The change is to which definition names an element when several match it.

```yaml
# Before: the general definition must exclude every special case by hand.
components:
  - name: Text
    selector: 'span:not(.label):not(.hint)'
  - name: Label
    selector: span.label
  - name: Hint
    selector: span.hint

# After: the special cases win on their own.
components:
  - name: Text
    selector: span
  - name: Label
    selector: span.label
  - name: Hint
    selector: span.hint
```

### Semantics

**More specific.** Selectors are ordered by what they *require* first and what they *exclude* second. Write `pos(S)` for S with every `:not()` removed. A is *more specific* than B when

- `pos(A)` implies `pos(B)` and `pos(B)` does not imply `pos(A)`; or
- `pos(A)` and `pos(B)` imply each other, and A's negations cover B's but not the reverse: at each aligned compound, every `:not(y)` of B has a `:not(x)` of A such that y implies x (whatever x excludes covers whatever y excludes).

Negations come second because they describe the other definitions an author had to fence off, not the element: `span[tabindex]` is the more specific description of a focusable span than `span:not(.label)`, though neither implies the other, and resolving in its favour is what lets the `:not()` go.

*Implies* is decided by the following procedure; it is syntactic and incomplete by design, recognising implications it can read off the selector text and never asserting one that does not hold. A selector is a chain of compounds joined by combinators. A implies B when B's chain **embeds** in A's from the subject outward:

1. A's subject compound implies B's subject compound.
2. Each further compound of B, moving toward the root, implies-aligns with some compound of A to the left of the previous alignment. Where the combinator before B's compound is a child combinator (`>`), the aligned compounds of A must be adjacent and joined by `>`; where it is a descendant combinator (whitespace), any compounds of A may lie between, as in CSS (`label span` is implied by `label > div span`).
3. Compounds of A left over beyond B's first are ancestor constraints B does not have; they only narrow.

Compound a implies compound b when b names no type (or `*`) or a names the same type, and every other simple selector of b is implied by some simple selector of a, where:

- `#x` is read as `[id="x"]` and `.c` as `[class~="c"]`.
- An attribute test on one name implies another on the same name by this table, comparing values exactly (a browser's case-insensitive attributes get no special treatment, which only ever withholds an implication):

  | a has | implies b's | when |
  |---|---|---|
  | any test on `n` | `[n]` | always |
  | `[n=v]` | `[n=w]` | `v = w` |
  | `[n=v]`, `[n^=v]`, `[n\|=v]` | `[n^=p]` | `v` starts with `p` |
  | `[n=v]`, `[n$=v]` | `[n$=s]` | `v` ends with `s` |
  | `[n=v]`, `[n^=v]`, `[n$=v]`, `[n*=v]`, `[n~=v]`, `[n\|=v]` | `[n*=s]` | `v` contains `s` |
  | `[n~=v]` | `[n~=w]` | `v = w` |
  | `[n=v]` | `[n~=w]` | `w` is one of `v`'s whitespace-separated tokens |
  | `[n\|=v]` | `[n\|=w]` | `v = w` |
  | `[n=v]` | `[n\|=w]` | `v = w`, or `v` starts with `w-` |

- A `:not()` on b is covered as defined above (its argument is a selector; y implies x by this same procedure).
- Any other pseudo-class on b (`:has()`, `:is()`, `:where()`) is implied only by an identical one on a.

**Resolution.** When several definitions match one element, every definition that some other matching definition is strictly more specific than is set aside. If one definition remains, it names the element. If more than one remains, the element is ambiguous: an SDK names it by whatever order it used before this SEP (the reference implementation keeps declaration order, after the chain-beats-global precedence it already applied) and SHOULD surface the ambiguity, as the reference `capture` and `observe` commands do under `[Conflicts]`.

The comparison is between the selectors that matched. A definition with several alternatives is compared by the alternative that matched the element, and a child definition by its whole chain (parent selectors joined to its own), which is why a scoped child is more specific than a bare global, and a child more specific than its own parent's definition when that reaches the same element (`label span` and `label span span[data-testid="x"]`).

View-scoped and global precedence is unchanged: a global whose name a view component shares is still replaced before matching. A global of another name remains a claimant, so it names an element that a less specific view component also reaches; between definitions the procedure cannot order, the view component still comes first.

This changes what names an element, never what matches. Coverage, tags (a union across every matching definition) and privacy (nearest declaration wins, `block` absolute) see every match as before.

### Conformance

- MUST name an element claimed by several definitions by the one that is strictly more specific than each of the others, when there is one, using exactly the procedure above.
- MUST NOT assert an implication the procedure does not (`[n^=""]`, `[n$=""]` and `[n*=""]` match nothing, so nothing implies them); two SDKs given the same corpus and tree MUST agree on which pairs are ordered.
- SHOULD surface an element whose remaining claimants number more than one.
- MUST leave tag and privacy resolution unchanged.

Fixture `go/conformance/fixtures/015-most-specific-wins` covers class, attribute-operator and chain ordering, and an unordered pair that stays ambiguous.

## Alternatives considered

- **CSS specificity, then declaration order.** Specificity counts simple selectors; it does not read them. `[a^="values-6-collection-values-"][a$="-action-label-text"]` and `[a^="values-"][a$="-text"]` tie at (0,2,0), as do `[class*="stepBadgeWrap"]` and `[class*="stepBadge"]`, so the cases that motivate this SEP fall to declaration order, and between unrelated definitions a higher count picks an arbitrary winner where an ambiguity should be reported.
- **Most simple selectors wins.** The same arbitrariness without the standard behind it.
- **Refuse ambiguity, as messages do (SEP-0006).** Right for messages, where a consumer can decline to classify a record. An element in a snapshot has to be called something, and the nested case has an answer every author would give.
- **Pure implication, negations included.** The first draft of this SEP ordered A over B only when A implied B outright. That never orders `span[tabindex]` against `span:not(.label)`, so the moment a general definition carries one `:not()` for a sibling the rule cannot resolve, every other sibling needs a `:not()` too, and nothing is gained. Requirements first, exclusions second, keeps the order sound (a definition never loses to one requiring less) and makes the `:not()` removable.
- **Declaration order, with authors (and generators) putting more specific definitions first.** The spec would still say nothing, other SDKs could differ, tooling would still report the pair as a conflict, and a child's position under `children:` carries document order, which the generator would then have to fight.
- **Do nothing.** Authors keep writing `:not()` chains that grow with every sibling, generators emit selectors the portable profile rejects, and the overlap stays reported as a conflict that is not one.

## Migration

No existing sightmap changes meaning unless it has an element claimed by two definitions, one strictly more specific than the other, and declared the less specific first. Such a corpus had a reported conflict on every such element; after this SEP the element goes to the more specific definition and the conflict is gone. An author who wanted the other to win could not have said so before either, and can keep it by making the two unordered (adding a test the more specific lacks) or by removing the overlap.

Tooling:

- The reference matcher applies the rule in `Match`, `Conflicts` and `NamesForChain`.
- `:not()` clauses whose only purpose was to exclude a more specific sibling can be removed; nothing requires it.

## Open questions

- **Children scoping.** This SEP changes what names an element, not what matches, so a definition's `children:` are still matched beneath every element its selector reaches, including one a more specific definition names. Should they instead be scoped to the elements the definition names? That reads naturally (a `Label`'s contents are not a `Text`'s) but makes child matching depend on resolution, and the NFA would have to drop a chain's in-progress states at a node its definition lost.
- Whether the validator should warn on a `:not()` that the rule makes redundant.

## References

- [SEP-0006](0006-message-entity.md): ambiguity between messages is refused rather than resolved.
- [SEP-0021](0021-portable-selector-profile.md): the selector profile whose limits long `:not()` chains exceed.
- [View matching](../v1/schema.md#view-matching-most-specific-wins): the most-specific-wins rule for routes.
