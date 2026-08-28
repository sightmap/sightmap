---
sep: 0003
title: Component property extraction via `properties[]`
author: Joel Webber (@joelgwebber)
status: Accepted
created: 2026-05-27
updated: 2026-07-27
spec-version-target: 1
related-issues: [52]
related-discussions: []
---

> **Superseded in part by [SEP-0010](0010-tree-closed-component-properties.md).**
> The `extract` grammar and live-DOM extraction model described below are
> replaced by SEP-0010, which resolves property values over the component tree
> (offline, cross-platform) and removes `transform`. The property *declaration
> shape* introduced here — `properties[]` with `name`/`extract`, the reserved
> `value` name, and silent omission — carries forward unchanged.

## Summary

Add an optional `properties: Property[]` field to `Component` entries. Each
property declares a named DOM extraction — a value pulled from the matched
element at snapshot time — that appears alongside the component name in
enriched snapshot output. A `[DateFilterButton label="This Weekend"]`
annotation tells an agent not just *what* component matched, but *what state
it is in*, without the agent having to parse raw accessible-name strings.

## Motivation

Today a sightmap corpus names components but says nothing about their runtime
state. A snapshot of a search results page might show:

```
[DateFilterButton]
[SortDropdown]
[PriceRangeFilter]
```

An agent reading this knows the filters exist but must dig into the raw AX
tree — verbose, full of structural noise — to find out whether "This Weekend"
is selected or whether the price range is "$50–$150". That context is exactly
what matters for structured event attribution (`click [DateFilterButton
label="This Weekend"]`) and for agents reasoning about what the user is
seeing.

Extracting property values closes this gap. Once properties are defined, the
same snapshot becomes:

```
[DateFilterButton label="This Weekend"]
[SortDropdown value="Most Popular"]
[PriceRangeFilter min="$50" max="$150"]
```

The value is extracted from the live DOM at the point the snapshot is taken,
encoded in the annotation, and from then on travels with the snap file as
structured data — no raw AX parsing needed by the consumer.

Real cases where this matters today:

- **Ticket listing pages** — each row needs `date`, `venue`, `price` to be
  attributable. Without properties, event rows collapse to bare link text.
- **Filter bars** — which filters are active? `label="active"` vs
  `label="inactive"` tells you; the a11y tree doesn't.
- **Nav links** — `label="Explore"` is far more useful than the AX name
  when the same component type appears six times in a nav rail.
- **Carousel / tab panels** — `label="1 / 4"` or `aria-selected="true"` can
  be surfaced as a property without custom extraction logic.

## Proposal

### Shape

A `Component` entry gains an optional `properties` array. Each entry in the
array is a `Property` object with a `name`, an `extract` directive, and an
optional `transform`.

```yaml
# Before — component with no property extraction
- name: DateFilterButton
  selector: '[data-testid="date-filter-button"]'

# After — component with declared properties
- name: DateFilterButton
  selector: '[data-testid="date-filter-button"]'
  properties:
    - name: label
      extract: text

- name: ShowDateRow
  selector: 'a[data-testid^="show-date-row-"]'
  properties:
    - name: date
      extract: inner_text          # layout-aware; avoids inline-element concatenation
    - name: sold_out
      extract: 'exists:[aria-label="Sold Out"]'   # omitted when absent

- name: ProductCard
  selector: '[data-component^="product-pod:ProductPod"]'
  properties:
    - name: price
      extract: '[data-testid="price"]'   # CSS sub-selector → textContent of child
      transform: first_dollar
    - name: rating
      extract: attr=aria-label
      transform: first_number
```

### Property field reference

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | yes | The key used in the snapshot annotation: `name="value"`. Must be a valid identifier (`[a-z][a-z0-9_]*`). |
| `extract` | string | yes | How to extract the value from the matched DOM element. See [Extract modes](#extract-modes) below. |
| `transform` | string | no | Optional post-processing applied after extraction. See [Transforms](#transforms) below. |

### Extract modes

The `extract` value is interpreted as follows, in this order:

| Value | Source |
|---|---|
| `text` | `element.textContent.trim()`, internal whitespace collapsed to single spaces |
| `inner_text` | `element.innerText.trim()` — layout-aware; adds whitespace between adjacent inline elements. Use when `text` produces run-together strings (e.g. date+time+venue concatenated without spaces). |
| `text_only` | Clone the element, remove all `img`, `svg`, and `[alt]` descendants, then `textContent.trim()`. Use when icon alt-text bleeds into the label. |
| `inner_html` | `element.innerHTML` |
| `attr=NAME` | `element.getAttribute(NAME)`. `NAME` is the attribute name after the `=`. |
| `exists:SEL` | `"true"` if `element.querySelector(SEL)` finds a match; the property is **omitted entirely** when the selector finds no match. Use for boolean state flags. |
| *any other string* | Treated as a CSS sub-selector: `element.querySelector(VALUE)?.textContent.trim()`. The property is omitted if the sub-selector matches nothing. |

The named modes (`text`, `inner_text`, `text_only`, `inner_html`) are matched
by exact, case-sensitive string equality before the prefix checks for `attr=`
and `exists:`. A value of `attr=text` selects the DOM attribute named `text`,
not the `text` mode.

### Transforms

When `transform` is specified, it is applied to the raw extracted string after
trimming. If the raw value is empty or absent, the transform is skipped and the
property is omitted.

| Value | Effect |
|---|---|
| `first_word` | First whitespace-delimited token |
| `last_word` | Last whitespace-delimited token |
| `first_number` | First substring matching `/\d[\d,.]*/` |
| `first_dollar` | First substring matching `/\$[\d,.]+/` |
| `number` | Strip all non-digit, non-decimal characters |
| `slug` | Lowercase; spaces and underscores → hyphens; strip non-`[a-z0-9-]` |

Transforms are not composable in v1 — only one may be specified per property.

### Semantics

#### Extraction scope

Extraction operates on the **exact DOM element** that the component's selector
matched — not on the AX node, and not on any ancestor. If the component matched
via a descendant combinator (`SiteHeader a`), extraction operates on the `a`
element, not the `SiteHeader` container.

#### Value omission

A property value is omitted from the annotation when:

- `extract` yields an empty string after trimming
- `extract` is `exists:SEL` and the sub-selector finds no match
- `extract` is a CSS sub-selector and the sub-selector finds no match

Omission is silent — no error or warning. The annotation simply carries fewer
key-value pairs. Consumers MUST NOT treat omission as an error.

#### Live-DOM requirement

Properties are extracted from the live DOM at snapshot time. SDKs that
operate purely on saved component-tree data (e.g. offline coverage tools)
MUST omit property values rather than approximate them. The presence of
`properties:` declarations in the corpus is not an error in offline mode;
the values are simply unavailable.

### Enriched snapshot output

Conforming SDKs SHOULD surface extracted property values alongside the
component name in enriched snapshot output, so that agents and analysts can
read the state of a matched component without parsing raw AX node text. The
exact output format — including how the component name is delimited, the
ordering of properties, and whether property values are quoted — is
implementation-defined and outside the scope of this SEP.

The one normative rule: if a component declares a property with `name: value`,
the sightmap-extracted value MUST take precedence over the accessibility tree's
own `Value` field for that component when both are present.

A separate SEP will define the canonical snapshot output format.

### JSON Schema diff

The `$defs.component` object gains one new optional property:

```
$defs.component.properties.properties:
  type: array
  items: { $ref: "#/$defs/componentProperty" }
  description: "Ordered list of DOM-value extractions surfaced in enriched snapshot output."
```

New `$defs` entry:

```
$defs.componentProperty:
  type: object
  required: [name, extract]
  additionalProperties: false
  properties:
    name:
      type: string
      pattern: "^[a-z][a-z0-9_]*$"
      description: >
        Key used in the snapshot annotation. The name "value" is reserved —
        it is valid in a properties[] declaration (overriding the AX built-in),
        but SDKs MUST treat "value" as the AX-value fallback even when no
        explicit declaration exists.
    extract:
      type: string
      minLength: 1
      description: "Extraction directive. See SEP-0003 §Extract modes."
    transform:
      type: string
      enum: [first_word, last_word, first_number, first_dollar, number, slug]
      description: "Optional post-processing applied to the extracted string."
```

`children` already recurses through `$defs.componentOrRef`, so child
components inherit the ability to declare `properties:` without schema change.

## Alternatives considered

### 1. Encode properties as runtime requests rather than static declarations

Properties could be specified as part of the `requests:` system (a separate
top-level concept) rather than baked into component definitions. This would
keep component entries purely structural and decouple extraction logic.

Ruled out: extraction is inherently tied to a specific component's matched
element — it's not a general-purpose request. Co-location in the component
definition is the natural place for authors to declare "what I want to know
about this element." Requests are for network-level signals; properties are for
DOM-level ones.

### 2. Support composite transforms (pipeline of multiple transforms)

Allow `transform: [first_dollar, number]` to strip the dollar sign and produce
a numeric string in one step.

Deferred, not rejected. The single-transform limit keeps the schema simple
for v1 and covers the overwhelming majority of real cases. Composition can be
added via a future SEP once the usage patterns are clearer.

### 3. Derive property name from the extract directive automatically

For `attr=aria-label`, automatically name the property `aria-label`. For
`text`, name it `text`. This would allow a shorthand like `extract: attr=href`
without a `name:`.

Ruled out: auto-derived names would be brittle (changing the `extract`
directive renames the property, breaking downstream consumers) and not always
meaningful (two different `attr=` extractions on the same component would
collide). Explicit naming is one extra line; the clarity is worth it.

### 4. Allow extraction from ancestor or sibling elements

Let `extract` reference a path outside the matched element's subtree, e.g.
`ancestor:[data-testid="shelf"] > text` to surface the shelf title alongside
each item card.

Ruled out for v1. Ancestor extraction requires a DOM traversal model beyond
what `querySelector` provides, adds significant implementation complexity, and
is usually solvable by promoting the ancestor to its own component. Out-of-scope
extraction can be revisited as a future SEP if the demand is clear.

## Migration

`properties:` is purely additive — existing sightmaps without the field are
fully valid under the updated schema. No corpus migration is required.

Existing SDKs that encounter a `properties:` entry MUST treat it as an unknown
field. Under `additionalProperties: false`, this means **existing SDKs will
reject sightmaps that use `properties:`** — the same constraint that applies to
every additive field under the current schema policy (see `spec/VERSIONING.md`
and the coordinated-release section of SEP-0001).

The release playbook follows the same pattern as SEP-0001:

1. This SEP merges.
2. `@sightmap/sightmap` re-vendors the schema and implements extraction in
   its snapshot pipeline. Bumped to a minor release.
3. All lockstep monorepo packages bump in the same changeset.
4. Adopters pin `@sightmap/sightmap >= <new version>` before adding
   `properties:` to any corpus.

## Open questions

1. **Property names on `$ref`-expanded components.** If a component with
   `properties:` is referenced via `$ref:`, the expanded copy inherits its
   properties. Is there a use case for *overriding* a property in the reference
   site, e.g. to change the extract mode for a specific view? No override
   mechanism is proposed in v1; the entire definition is deep-copied.

2. **Property values in structured event paths.** Property values would
   naturally appear in structured event attribution paths
   (`click [ShowDateRow date="Fri Aug 21"] > [FindTicketsButton]`), but the
   event attribution format is not yet specified. Left for a future SEP.

3. **Multi-match extraction.** When a component's selector matches multiple
   elements (e.g. `CategoryTile` matching 12 tiles), extraction runs
   independently on each matched element. This is correct and expected, but
   conformance fixtures should verify that each matched element carries its
   own property values, not the first-matched element's values.

## References

- SEP-0002 (`$ref`) — the cross-view component-reference mechanism this field
  composes with
- sightmap/go go-0025 — reference implementation of property extraction
  (lidar-id-anchored DOM lookup, batched EvalJSON)
- Playwright Locator API — prior art for element-scoped `textContent` vs
  `innerText` semantics
