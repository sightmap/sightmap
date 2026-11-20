---
sep: 0026
title: App view and Text nodes in native trees
author: Chandler Van De Water (@chandlervdw)
status: Draft
created: 2026-11-02
updated: 2026-11-02
spec-version-target: 1
related-issues: []
related-discussions: []
---

## Summary

Name two kinds of node that a native component tree MAY carry when the
implementation can see inside the app, and the attributes corpora read from
them. A **`view`** node stands for an instance of an app-defined view type: its
`type` attribute is the type's name, and its other attributes are that
instance's stored fields as dotted paths (`item.title`). A **`text`** node
inside it stands for one piece of displayed text, with `field` naming the
stored field it shows, and `value` / `format` for a value the app formatted.
With these, a corpus reads title, maker and price from a list row whose
accessibility label joins them, exactly and without patterns. Nothing in the
YAML schema changes; a tree that lacks these nodes still matches every corpus
as it does today.

## Motivation

On iOS, SwiftUI combines the Texts inside a `NavigationLink` or `Button` into
one accessibility element whose label joins them with ", ":

```swift
NavigationLink(value: item.id) {
    CatalogRow(item: item.summary, maker: item.maker)   // HStack { Text(title); Text(maker); Text(price, format: .currency) }
}
.accessibilityIdentifier("catalog-row-\(item.id)")
```

```
button[identifier="catalog-row-1"] "Socks, Wool, Pair of 2, Acme, Inc., $9.99"
```

That is everything an out-of-process accessibility reader gets (the reference
CLI's `sightmap device`, XCUITest, idb). A corpus that wants the title, maker
and price has only one tool, a pattern over `dom.text`:

```yaml
- name: title
  extract: { from: dom.text, pattern: '^(.+?)(?:, [^,]+)?, \$[\d,]+(?:\.\d+)?$' }
```

and that tool cannot work in general. The separator is ", ", values contain
", " (a title "Socks, Wool, Pair of 2", a maker "Acme, Inc."), and no pattern
can tell which comma separates fields. The price is the formatted string, so
a corpus that wants a number has to re-parse a locale-specific currency
format.

An implementation that runs code inside the app (the reference CLI's
`device start --probe`, which reads SwiftUI's view debug graph in the iOS
Simulator) can see the structure the app was written with: the `CatalogRow`
instance, its stored fields, and each Text with its own string or unformatted
value. The question this SEP answers is how that structure appears in the
component tree, so that corpora written against it mean the same thing in
every implementation that produces it, and degrade cleanly in those that don't.

## Proposal

### Shape

No YAML schema change. The SEP fixes a tree vocabulary that existing
selectors and extract sources already reach.

A tree with the nodes (the reference CLI's snapshot rendering):

```
4 button[identifier="catalog-row-1"] "Socks, Wool, Pair of 2, Acme, Inc., $9.99"
  5 view[type="CatalogRow"]                       item.id="1" item.title="Socks, Wool, Pair of 2" item.price="9.99" maker="Acme, Inc."
    6 text[field="item.title"] "Socks, Wool, Pair of 2"
    7 text[field="maker"] "Acme, Inc."
    8 text[field="item.price"][format="currency USD"] "9.99" value="9.99"
```

A corpus that reads it:

```yaml
# Before: patterns over the joined label (wrong whenever a value contains ", ")
- name: CatalogRow
  selector: '[identifier^="catalog-row-"]'
  properties:
    - name: title
      extract: { from: dom.text, pattern: '^(.+?), [^,]+, \$[\d.]+$' }

# After
- name: CatalogRow
  selector: '[identifier^="catalog-row-"]'
  properties:
    - name: title
      extract: { from: component, path: Title.text }
    - name: maker
      extract: { from: component, path: Maker.text }
    - name: price
      extract: { from: component, path: Price.value }
  children:
    - name: Title
      selector: 'text[field="item.title"]'
      properties: [{ name: text, extract: { from: dom.text } }]
    - name: Maker
      selector: 'text[field="maker"]'
      properties: [{ name: text, extract: { from: dom.text } }]
    - name: Price
      selector: 'text[field="item.price"]'
      properties: [{ name: value, extract: { from: dom.attr, path: value } }]
```

**JSON Schema diff:** none.

### Semantics

**`view` node.** Tag and role `view`. Stands for one instance of a view type
the app defines (not one from the platform's UI framework). Attributes:

| Attribute | Value |
|---|---|
| `type` | The type's unqualified name without generic arguments (`CatalogRow`, not `MyApp.CatalogRow<EmptyView>`). |
| *field path* | One attribute per scalar stored field of the instance, named by its dotted path from the instance (`maker`, `item.title`, `item.price`). Numbers in their shortest round-trip decimal form (`9.99`, `12`), booleans `true`/`false`. A field named `type` is not carried. |

A `view` node is never interactive and never the target of an action by
itself; its bounds are those of the element it sits under, or its own when the
implementation knows them. It sits under the element that represents it on
screen (the row's button above), and the `text` nodes it renders are its
children.

**`text` node.** Tag and role `text`, as in any tree. Inside a `view`, or
directly under an element whose label joins several texts:

| Attribute | Value |
|---|---|
| `text` (and the node's name) | The text's own string. For a formatted value, the unformatted value (below). |
| `field` | The field path of the enclosing `view` whose value the text shows, when exactly that value is shown. Absent for computed or literal text. |
| `value` | For text that displays a value through a formatter (`Text(9.99, format: .currency(code: "USD"))`), the unformatted value. |
| `format` | For such text, a short name of the formatter: `currency <ISO code>`, `number`, `percent`, `date`. |

**Placement.** An implementation adds these nodes under the existing element
they belong to. It MUST NOT remove or re-parent the elements it would report
without them, and it MUST NOT add a `text` node whose string an existing
element already reports separately (as its own label or a child's), so a tree
never shows the same text twice. Children are ordered by position on screen
(top to bottom, then leading to trailing), not by declaration.

**Privacy.** The stored fields of a view can hold more than the screen shows.
An implementation MUST apply its credential redaction to field attributes
(a field whose path has a segment naming a token, password, secret or key
carries `<redacted>`; a URL's credential query parameters are redacted) and to
a `text` node whose `field` is such a field. It MUST NOT add nodes under a
secure text field. Component privacy rules (SEP-0009) apply to these nodes
like any others. An implementation SHOULD let the user leave field attributes
out (keeping `type` and the `text` nodes).

### Conformance

- An implementation MAY produce `view` and `text` nodes. Producing them is
  optional; a corpus that relies on them MUST still load and match (with the
  dependent properties absent) in an implementation that doesn't.
- When it produces them, it MUST follow the attribute names and value forms
  above, the placement rules and the privacy rules.
- It MUST NOT use the `view` tag for anything else.
- Matching and extraction are unchanged: `view` and `text` nodes are ordinary
  nodes for selectors, `from: dom.text`, `from: dom.attr` and child
  components.

## Alternatives considered

**Split the joined label.** An implementation could split the accessibility
label on ", " into child texts. It can't be right: the separator occurs inside
values, and it would invent structure the app never declared. Ruled out.

**Ask apps to keep fields apart.** `accessibilityCustomContent` (iOS) lets an
app attach labelled values ("Maker": "Acme, Inc.") to the row for VoiceOver,
and it does cross the simulator's accessibility bridge, so an out-of-process
reader could expose it as attributes. That is worth supporting as well, but it
needs a change in every app, gives only the formatted strings, and says
nothing about the view type. It complements this SEP rather than replacing it.

**Expose the platform's whole view graph.** SwiftUI's debug graph has ~180
nodes for a three-Text row, almost all framework wrappers whose names change
between releases. Putting it in the tree would make corpora depend on
framework internals. This SEP keeps only the two things authors write: their
own view types and their Texts.

**Do nothing.** Corpora keep patterns over joined labels and are wrong for any
value containing the separator.

## Migration

None required. Existing corpora are unaffected: no implementation produced
`view` nodes or the `field` / `format` attributes before. A corpus can move
from label patterns to child components when its implementation produces
these nodes; it degrades to absent properties, not errors, where they are not
produced.

## Open questions

- Whether `type` should be qualified by module when two modules define the
  same name.
- Field paths for collections (`tags.0`), which the reference implementation
  does not emit yet.
- Whether `view` nodes should also appear for app views that have no Text of
  their own, as structure only. The reference implementation adds them only
  when they carry texts.
- Android (Compose) equivalents: composable names and their parameters are
  the analogue; nothing reads them yet.

## References

- SEP-0009 (component privacy), SEP-0013 (richer node data), SEP-0024 (route-less views, the native tree's vocabulary).
- Reference implementation: `sightmap device start --probe` (`go/device/probe*.go`, `go/device/probe/probe.m`).
- Apple: [`accessibilityCustomContent`](https://developer.apple.com/documentation/swiftui/view/accessibilitycustomcontent(_:_:importance:)-2yx1b).
