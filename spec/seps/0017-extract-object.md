---
sep: 0017
title: One extract object across components, requests and messages
author: Clint Ayres (@jurassix)
status: Review
created: 2026-10-02
updated: 2026-10-02
spec-version-target: 1
related-issues: [443]
related-discussions: []
---

## Summary

Replace the per-entity extraction grammars with one object:

```yaml
extract:
  from: rsp.body        # a source, from one shared namespace
  path: order.ref.id    # the value within it
  pattern: '^ORD-(\d+)' # optional RE2 refinement

extract:
  from: component
  path: 'Tag[].value'   # Tag[] reads every match, not the first
  join: ','             # collapses the matches into one value
```

Every entity draws `from` values from one namespace; which values an entity accepts is a property
of the entity, not of the syntax. The existing string forms are lowered to the object, keep working
for a deprecation window, and are then removed. This SEP changes how extraction is written and adds
two refinements to components (`pattern`, and `join` over a `Name[]` path); it does not change what a
source resolves to.

## Motivation

Extraction reached the spec several times, and each entity got its own micro-syntax because each
was designed on its own:

| Entity | Reads from | Spelled today |
|---|---|---|
| Component | the matched node and its component tree | `extract: text`, `attr=NAME`, `PATH.prop`, `exists:PATH`, `raw_text` |
| Request | a body or header block | `source: rsp.body` + `field: order.ref.id` + `pattern:` |
| Message | an exception's call stack | `source: stack` + `field: top.file` + `pattern:` |

An author learns three spellings for one operation. A consumer that reasons about extraction across
entities special-cases each one. Every new source means inventing another spelling rather than
adding one value to a shared list, and open proposals were each about to add one: `query:` and
`param:` for URLs, a `pattern` suffix for components, a predicate and multi-match syntax for paths.

The string forms also hide a collision. SEP-0013 accepted reading a control's interactive state with
`attr=checked`, which would make `attr=` read the accessibility layer for four names and the DOM for
every other. One `from` per store removes the special case.

## Proposal

### Shape

```yaml
extract:
  from: <source>     # required
  path: <string>     # required or forbidden, per source
  pattern: <regex>   # optional
  join: <string>     # optional, component reads only
```

`extract` is an object on every property entry that extracts a value: `components[].properties[]`,
`requests[].properties[]` and `messages[].properties[]`. No other keys are allowed in it.

### Sources

| `from` | Resolves to | `path` | Valid on |
|---|---|---|---|
| `dom.text` | the node's accessible text | forbidden | component |
| `dom.raw_text` | the node's own direct text-node content, whitespace-normalized (SEP-0013) | forbidden | component |
| `dom.attr` | the value of a carried DOM attribute, as the markup states it | attribute name | component |
| `dom.state` | the node's current interactive state (SEP-0013) | `checked`, `selected`, `disabled` or `expanded` | component |
| `component` | a descendant component's own extracted property (SEP-0010) | `Comp(.Comp)*.prop` | component |
| `component.exists` | `"true"` when a descendant component path matches; omitted otherwise | `Comp(.Comp)*` | component |
| `req.body`, `rsp.body` | a value in a parsed body (SEP-0005) | object-key path; optional with `pattern` | request |
| `req.headers`, `rsp.headers` | a header, matched case-insensitively (SEP-0005) | header name | request |
| `stack` | a frame attribute of an exception's call stack (SEP-0006) | `<frame>.<attribute>` | message |

`url.query` and `url.path` are reserved for URL-shaped properties (SEP-0008). A `from` that is not
in the table, or is not valid on the entity it appears on, is an error.

Each source resolves exactly as the SEP that introduced it defines; only the spelling moves. Two
clarifications fall out of giving each store its own name:

- **`dom.attr` reads the markup.** `from: dom.attr, path: checked` is the `checked` attribute as
  authored, which records a control's initial state. It never reads interactive state.
- **`dom.state` reads interactive state.** It returns `"true"` or `"false"` (`"mixed"` for an
  indeterminate checkbox) for a node that has that state and is omitted otherwise. It replaces the
  `attr=` spelling SEP-0013 accepted, with the same values and the same carriage requirement.

### Refinements

**`pattern`** is an RE2 regex applied to the resolved value. Capture group 1 is the result when the
pattern has one, otherwise the whole match. A value the pattern does not match is omitted. For a
body source with no `path`, the pattern scans the raw body text, as SEP-0005 already allows.
`pattern` is valid on every source except `component.exists`, whose result is not text.

Requests and messages already carry `pattern`. Components gain it here: a price inside a label such
as `Add to cart · $10.95` is reachable without promoting a sub-element to a child component.

**Multi-valued paths and `join`.** A component path states its shape. Each segment resolves within
the node or nodes the previous segment produced, in document order:

- A plain segment, `Tag`, takes the **first** match, as SEP-0010 defines. A path of plain segments
  is single-valued and yields one value.
- A segment written **`Tag[]`** takes **every** match. A path with one or more `[]` segments is
  multi-valued, and resolves to one value per node it reaches.

`Row[].Price.amount` reads the first `Price` in every `Row`; `Row.Price[].amount` reads every
`Price` in the first `Row`; `Row[].Price[].amount` reads every `Price` in every `Row`.

A multi-valued path requires **`join`**, which collapses it into one value: each value is read (and
refined by `pattern`, when present), empty values are dropped, and the rest are concatenated with
the `join` string. No surviving value omits the property. `join` is valid only with
`from: component`, must be non-empty, and requires a `[]` segment to collect. A multi-valued path
without `join` is reserved for array-valued results, which a later SEP may define.

```yaml
- name: ProductCard
  selector: '.product'
  properties:
    - name: first_tag
      extract: { from: component, path: Tag.value }                 # "sale"
    - name: tags
      extract: { from: component, path: 'Tag[].value', join: ',' }  # "sale,new,featured"
  children:
    - name: Tag
      selector: '.tag'
      properties:
        - name: value
          extract: { from: dom.text }
```

`[` and `]` are flow indicators in YAML, so a path containing `[]` must be quoted inside a flow
mapping (`{ ... }`), as above. In block style it needs no quotes.

**Predicates are reserved.** Picking one match by what it is, such as the selected tab
(`Tab[selected=true].label`), needs a predicate on a path segment. This SEP does not define one. A
bracket with content inside a path segment is reserved for it and is rejected by validation, so a
predicate syntax along the lines of SEP-0012 can be added without changing any valid path.

A joined scalar is a value every consumer can store, where an array is not.

### Privacy

SEP-0009 governs extracted values, judged at the node a value is read from. Restated per source:

| `from` | Withheld when |
|---|---|
| `dom.text`, `dom.raw_text`, `dom.attr` | the node's effective privacy is `mask` or `block` |
| `dom.state` | the node's effective privacy is `block` |
| `component` | the target's own property is withheld, judged at the target node |
| `component.exists` | the target node's effective privacy is `block` |

A property that reads `dom.text`, `dom.raw_text` or `dom.attr` from a component whose own `privacy`
is `block` or `mask`, or `dom.state` from one whose own `privacy` is `block`, is withheld by every
capture consumer. Validation warns (`extract-privacy-withheld`) rather than letting it fail silently
at runtime. It is a warning, not an error, because a consumer that captures no content ignores
`privacy` (SEP-0009) and still resolves the value. The fix is to change the component's privacy, a
field a privacy review reads, so relaxing privacy for the sake of an extraction stays visible.

### String forms

The string forms remain valid for a deprecation window and lower exactly:

| Today | Object |
|---|---|
| `extract: text` | `{ from: dom.text }` |
| `extract: raw_text` | `{ from: dom.raw_text }` |
| `extract: attr=NAME` | `{ from: dom.attr, path: NAME }` |
| `extract: PATH.prop` | `{ from: component, path: PATH.prop }` |
| `extract: exists:PATH` | `{ from: component.exists, path: PATH }` |
| `source: S`, `field: F`, `pattern: P` | `{ from: S, path: F, pattern: P }` |

`attr=NAME` always lowers to `dom.attr`, including for the four state names. Reading state through
`attr=` was accepted by SEP-0013 but never shipped, so no corpus depends on it.

A property may use the object or a string form, not both: `extract` alongside `source`, `field` or
`pattern` on one entry is an error (`extract-shape-mixed`). Every string form draws a deprecation
warning (`extract-legacy-form`) naming its object equivalent.

### Conformance

A conforming SDK MUST:

- Accept the `extract` object on component, request and message properties, with the sources and
  `path` rules above, and reject an unknown key, an unknown `from`, or a `from` not valid on the
  entity.
- Resolve each source as its defining SEP specifies, `dom.state` as SEP-0013's interactive-state
  set, and `dom.attr` as the authored attribute.
- Resolve plain path segments to their first match and `Name[]` segments to every match, require
  `join` on a multi-valued path, reject `join` on a single-valued one, and reject a bracket with
  content in a path segment.
- Apply `pattern` on every source except `component.exists`.
- Apply SEP-0009 per the privacy table, and warn with `extract-privacy-withheld`.
- During the deprecation window, accept the string forms, lower them exactly as tabled, and warn with
  `extract-legacy-form`. Report `extract-shape-mixed` for a property mixing both.

A conforming SDK MUST pass the conformance fixtures this SEP adds and updates.

## Alternatives considered

- **Keep per-entity grammars and add new string forms as needed.** Rejected: each new source would
  add a spelling, and `attr=` would keep reading two different stores.
- **Picking one match with a narrower component** (an `ActiveTab` whose selector matches only the
  selected `Tab`). Rejected: one node matching both `Tab` and `ActiveTab` breaks the one-match-per-node
  model the rest of the design assumes. A predicate on the path is the right tool; its syntax is
  reserved here and deferred.
- **Collecting every match whenever `join` is present**, with no marker in the path. Rejected: the
  path should state whether it is single- or multi-valued, and in a multi-segment path a single
  `join` cannot say which segments fan out.
- **Allow `join` on every source.** Deferred. No other source on main produces a multi-match; a source
  that does (repeated query parameters, say) can enable it in its own SEP.
- **A hard cut with no deprecation window.** Rejected. Lowering is mechanical and exact, so a window
  costs little and lets corpora and consumers move on their own schedule.

## Migration

Every string form has exactly one object equivalent, so migration is mechanical, and the
`extract-legacy-form` warning names the replacement for each occurrence. The string forms are
removed in a later minor release, after at least one release that warns. Pre-1.0, per
[`VERSIONING.md`](../VERSIONING.md), that removal tightens spec stream `1` in place; the YAML
`version:` field does not change.

Go SDK consumers see `ComponentPropertyDef`, `RequestPropertyDef` and `MessagePropertyDef` carry one
`Extract` value in place of the per-entity fields, already lowered from whichever form the corpus
used.

## Open questions

- **Path predicates.** What a predicate inside `Name[...]` may test (a property value, a state, an
  attribute) and how it composes with `[]`. The syntax is reserved; SEP-0012 is the starting point.
- **Array-valued results.** A multi-valued path without `join` is reserved for a result that keeps
  each value separate, which consumers that store scalars cannot represent today.

## References

- SEP-0003, SEP-0010: component properties and the tree-closed extract grammar.
- SEP-0005: request properties. SEP-0006: message properties.
- SEP-0008: URL properties, which defines `url.query` and `url.path`.
- SEP-0009: component privacy. SEP-0013: `raw_text` and interactive state (issue #443).
