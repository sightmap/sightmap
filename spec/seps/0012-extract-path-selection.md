---
sep: 0012
title: Extract path selection — predicates and multi-match
author: Joel Webber (@joelgwebber)
status: Draft
created: 2026-09-08
updated: 2026-09-08
spec-version-target: 1
related-issues: []
related-discussions: []
---

## Summary

Extend the SEP-0010 extract `PATH` two ways: a segment may carry a **predicate**
(the `[prop op value]` grammar component queries already use) to choose which
match it resolves to, and a segment may carry an **all-match collector** `[]` that
resolves every match, making the property **array-valued**. These close the two
open questions SEP-0010 deferred: *path predicates* and *aggregates*.

## Motivation

SEP-0010 resolves each `PATH` segment to the **first** match in document order,
with no way to say which one or to take more than one. Two everyday shapes have no
expression today:

- **Which match.** A container holds several same-shaped children and the value
  you want is on the one in a particular state — the selected tab, the active row.
- **All matches.** A node legitimately has several values — a list of tags, a set
  of identifiers — and first-match silently drops all but one.

## Proposal

### Predicate on a segment

A `PATH` segment may be suffixed with a predicate `[prop op value]`, reusing the
component-query predicate grammar. The segment resolves to the first match, in
document order, whose declared property `prop` satisfies the predicate.

```yaml
- name: TabStrip
  selector: '[role="tablist"]'
  properties:
    - name: current
      extract: Tab[selected="true"].label   # the selected tab's label
  children:
    - name: Tab
      selector: '[role="tab"]'
      properties:
        - name: label
          extract: text
        - name: selected
          extract: attr=aria-selected
```

The predicate reads a **declared property** of the addressed child (itself
tree-closed), so the path stays offline-resolvable.

### All-match collector

A `PATH` segment may be suffixed with `[]` to resolve **every** match instead of
the first. A property whose path contains `[]` is **array-valued**: one entry per
match, in document order.

```yaml
- name: Article
  selector: 'article'
  properties:
    - name: tags
      extract: Tag[].label       # -> ["new", "sale"]
  children:
    - name: Tag
      selector: '.tag'
      properties:
        - name: label
          extract: text
```

At most one `[]` per path in v1. A predicate and a collector may combine on one
segment: `Row[state="active"][].amount` yields the amounts of every active row.

### Value model

A property value is a **string** (as today) or, when its path contains `[]`, an
**array of strings**. Consumers that render or return property values MUST accept
both shapes. A `pattern` (SEP-0011) applies per element.

### JSON Schema

- `$defs.componentProperty.extract`: the `PATH` grammar widens — a segment may
  carry a `[prop op value]` predicate and/or a trailing `[]`. (Grammar is
  validated by the SDK, per SEP-0010, not by JSON Schema.)
- The property **value** type widens from `string` to `string | array<string>`.

### Semantics

- **Tree-closed / offline (unchanged).** Predicate evaluation reads a declared
  property; collection walks the same matched subtree. No DOM reach.
- **Order is document order** at each segment.
- **Omission (unchanged).** A predicate that matches nothing, or a `[]` with no
  matches, omits the property; an empty array is not emitted.
- **One `[]` per path** (v1). Nested / multiple collectors are deferred.

### Conformance

A conforming SDK:

- MUST resolve a segment predicate against the addressed child's declared
  properties and select the first satisfying match in document order.
- MUST, for a path containing `[]`, resolve every match of that segment and yield
  an array of the per-match results in document order.
- MUST reject more than one `[]` in a single path as invalid corpus input.

## Open questions

- **Annotation + coverage of arrays.** How a list value renders in the snapshot
  (e.g. `[Article tags=["new","sale"]]`); coverage is containment-based so its
  tiering is unchanged, but the annotation shape needs a decision.
- **Multiple `[]`** (nested lists / cartesian) — deferred until a real case needs it.
- **Predicate operators** inherit the component-query set exactly; no new ops here.

## References

- **SEP-0010** — the extract `PATH` and the two open questions ("path predicates",
  "aggregates") this closes.
- **SEP-0011** — `pattern` capture, which applies per array element.
