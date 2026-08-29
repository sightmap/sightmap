---
sep: 0010
title: Tree-closed component property extraction
author: Joel Webber (@joelgwebber)
status: Accepted
created: 2026-08-27
updated: 2026-08-27
spec-version-target: 1
related-issues:
  - https://github.com/sightmap/sightmap/issues/282
related-discussions: []
---

## Summary

Redefine component `properties[]` extraction so it resolves against the abstract
component tree instead of the live DOM. Every `extract` directive references only
(a) the matched node's own carried data — its accessible text and its
attributes — or (b) the extracted property of a component nested beneath it. The
DOM-shaped component modes (`inner_text`, `text_only`, `inner_html`, bare-CSS
sub-selectors) are removed. Separately, `transform` is removed from **every**
property type — component, request, and message: the fixed transform vocabulary
was ill-conceived and inconsistent (an unspecced `match:` mode even leaked into
tooling), and request/message extraction already has `pattern` (RE2 + capture
groups) for the cases that matter. The tree-closed component change makes property
values resolvable offline, from a serialized tree, on any UI platform — closing
the gap where the only path that produced values was a live-DOM pass over CDP.

## Motivation

SEP-0003 defined `extract` in terms of `element.querySelector` / `innerHTML`, so
property values could only be produced against a live DOM. A consumer holding a
matched, serialized component tree with no browser — the offline evaluation half,
the component analogue of SEP-0005/0006 for requests and messages — cannot
resolve any property value. SEP-0003 acknowledged this by requiring offline SDKs
to *omit* values rather than approximate them, which leaves offline and
non-web consumers permanently unable to read component state.

The root cause is that extraction directives reach past what the abstract node
carries: a component tree is a deliberately lossy, platform-neutral projection
(it must also describe native iOS/Android hierarchies), so a directive like
`extract: '.price'` has nothing to resolve against once the DOM is gone. The fix
is to define extraction over the tree itself.

## Proposal

### Shape

**YAML — before/after.**

```yaml
# Before (SEP-0003): a DOM sub-selector, unresolvable offline / off-web
- name: FlowFormField
  selector: 'flowruntime-screen-field'
  properties:
    - name: label
      extract: '.slds-form-element__label'

# After: the sub-element is a declared child component; the parent
# references its extracted property.
- name: FlowFormField
  selector: 'flowruntime-screen-field'
  properties:
    - name: label
      extract: FlowFormLabel.label
  children:
    - name: FlowFormLabel
      selector: '.slds-form-element__label'
      properties:
        - name: label
          extract: text
```

Local text and attributes need no children:

```yaml
- name: RecentRecordLink
  selector: 'ul.recentsRecordCardList a'
  properties:
    - name: label
      extract: text
    - name: href
      extract: attr=href
```

Multi-value card with a boolean flag:

```yaml
- name: ProductCard
  selector: '[data-component^="product-pod:ProductPod"]'
  properties:
    - name: price
      extract: Price.text
    - name: sold_out
      extract: exists:SoldOutBadge
  children:
    - name: Price
      selector: '[data-testid="price"]'
      properties:
        - name: text
          extract: text
    - name: SoldOutBadge
      selector: '[aria-label="Sold Out"]'
```

### Extract grammar

`extract` is exactly one of:

| Form | Resolves to |
|---|---|
| `text` | the node's accessible text |
| `attr=NAME` | the value of attribute `NAME` carried by the node; omitted if the node does not carry it |
| `PATH.prop` | the value extracted for property `prop` of the descendant component addressed by `PATH` |
| `exists:PATH` | `"true"` if `PATH` resolves to at least one matched component; omitted otherwise |

`PATH` is a dotted sequence of component names naming a descendant, each segment
resolved within the previous segment's matched subtree
(`Price`, `Row.Price`). In a `PATH.prop` value reference the final segment is a
property name; in `exists:PATH` the whole path names components.

### JSON Schema diff

- `$defs.componentProperty`: remove the `transform` property entirely.
- `$defs.requestProperty` and `$defs.messageProperty`: remove the `transform`
  property entirely; their `source`/`field`/`pattern` extraction is unchanged.
- `$defs.componentProperty.extract`: unchanged as a type (`string`, `minLength:
  1`); its *accepted grammar* narrows to the four forms above. The named DOM
  modes `inner_text`, `text_only`, `inner_html` and the bare-CSS-sub-selector
  fallback are no longer valid values. (Grammar is validated by the SDK, not by
  JSON Schema.)

### Semantics

Resolution runs against the serialized component tree, with no live DOM, and
MUST run over the **complete, unfiltered** tree — a consumer that prunes the
tree (e.g. to interactive nodes) before resolving may drop referenceable
children.

- **`text`** is the node's accessible name/text as computed by the consumer's
  accessibility layer. What exactly that yields is implementation-defined.
- **`attr=NAME`** reads from the attribute set the consumer carries on the node.
  Which attributes are carried is implementation-defined (a web tool may carry a
  fixed allowlist plus `aria-*`/`data-*`; other platforms carry synthetic
  attributes). An attribute the consumer did not carry is indistinguishable from
  one that was absent: the property is omitted.
- **References descend only.** A property may address a component nested beneath
  the one declaring it, never a parent, sibling, or cousin. The resulting
  dependency graph is a DAG; a consumer resolves it bottom-up.
- **Multi-match resolves to the first match** in document order at each path
  segment.
- **Component names are unique per parent**, so a path segment resolves
  unambiguously within its parent's declared children.
- **Omission is silent** (unchanged from SEP-0003): a property whose `text` is
  empty, whose attribute is not carried, or whose `PATH` matches nothing is
  dropped from the annotation. Consumers MUST NOT treat omission as an error.
- **The reserved `value` property** is unchanged: a declared `value` MUST take
  precedence over the node's accessibility value.

### Conformance

A conforming SDK:

- MUST resolve `text`, `attr=NAME`, `PATH.prop`, and `exists:PATH` against the
  serialized component tree, without requiring a live DOM.
- MUST resolve references over the complete tree and MUST descend only.
- MUST omit a property that does not resolve, silently.
- MUST reject `transform` (on any property type) and the removed DOM component
  extract modes as invalid corpus input (validation error), rather than silently
  accepting them.
- MAY define what `text` and the carried attribute set contain for its platform.

## Alternatives considered

- **Do nothing (keep SEP-0003, offline omits).** Leaves every offline and
  non-web consumer unable to read component state — the exact gap in #282. The
  live-DOM extractor also cannot be reused off-web. Insufficient.
- **Enrich the serialized node with raw DOM (full text, all attributes,
  innerHTML).** Re-leaks the DOM into an abstraction that must also describe
  native hierarchies (`innerHTML` is meaningless there) and entrenches the
  web-only coupling. Rejected. (Carrying an abstract *text* field is fine and
  orthogonal; carrying HTML/arbitrary descendants is not.)
- **Two profiles: full DOM/CSS power on web, a reduced set off-web.** Makes
  offline and native permanently second-class and forces each consumer to
  interpret DOM directives however it can. Rejected in favor of one model that
  every platform resolves identically.

## Migration

Breaking, but additive-shaped and bounded — there are few corpora using
`properties[]`, and single-value label cases already collapse to `extract: text`
on the matched node.

- **Corpora** using a bare-CSS sub-selector, `exists:SEL`, `inner_text`,
  `text_only`, or `inner_html` must promote the referenced sub-element to a
  declared child component and reference it via `PATH.prop` / `exists:PATH`.
  Corpora using `transform` on **any** property type (component, request, or
  message) must drop it; for request/message, fold the extraction into `pattern`
  where possible (`first_number` → `(\d[\d,.]*)`, `first_dollar` → `(\$[\d,.]+)`,
  etc.). The two mutation transforms `number` and `slug` have no `pattern`
  equivalent, but are near-useless on request bodies and stack frames.
- **SDKs** replace the live-DOM extraction pass with tree resolution; the live
  path becomes "build tree → match → resolve over the tree," so one
  implementation serves both live and offline.
- **Tooling we ship:** update `spec/v1/schema.md` and
  `sightmap.schema.json` (drop `transform`, narrow the `extract` grammar
  description), rewrite the SEP-0003 examples and conformance fixtures, and add a
  validator/lint that flags removed extract modes and `transform` with the
  child-component rewrite as the suggested fix.

This SEP supersedes the extraction model of SEP-0003; SEP-0003's property
*declaration shape* (`name`/`extract`, the reserved `value` name, silent
omission) carries forward. It also removes the `transform` field from request
(SEP-0005) and message (SEP-0006) properties; the `source`/`field`/`pattern`
extraction those SEPs define is otherwise unchanged.

## Open questions

- **Path predicates.** Should a path segment be able to carry a predicate
  (`Row[state=active].Price.amount`) to disambiguate among multiple matches?
  Deferred until a real case needs more than first-match.
- **Aggregates.** `count` / `sum` over a multi-match child path — deferred.
- **Richer `text`.** Whether an abstract `raw_text` (raw text content, distinct
  from the accessibility name) is worth carrying for markup-noise cases.
  Deferred; both current probes already compute it, so it is recoverable later.

## References

- SEP-0003 (component `properties[]`) — the declaration shape this supersedes the
  extraction model of.
- SEP-0005 / SEP-0006 — the request/message property SEPs; this SEP removes their
  `transform` field (issue #170 is the offline-evaluation precedent this
  parallels).
- Issue #282 — the missing offline component-property evaluation half.
