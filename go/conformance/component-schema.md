# ComponentNode JSON Schema

This document is the authoritative contract for the `ComponentNode` JSON shape.
Both the Go library (`comps.ComponentNode`) and the TypeScript sightmap/js
implementation must produce and consume this shape identically.

## SelectorPart

Represents the structured CSS identity of a DOM element (or a synthetic
identity for native mobile elements).

| Field | Type | JSON key | Description |
|---|---|---|---|
| Tag | string | `tag` | Lowercase element tag (`"div"`, `"button"`) or mobile type (`"UIButton"`) |
| Id | string | `id` | The `id` attribute value |
| Classes | []string | `classes` | CSS class list in document order |
| Attrs | map[string]string | `attrs` | Attribute key→value pairs |
| AttrOps | map[string]string | `attrOps` | Operator for each Attrs entry where not `"="`. See operators below |
| Not | \*SelectorPart | `not` | If non-nil, the element must NOT match this sub-selector (`:not()`) |

All fields are `omitempty` — absent from JSON when zero/nil.

### Attribute operators

| Operator | Meaning |
|---|---|
| `=` | Exact equality (default; omit from AttrOps) |
| `^=` | Value starts with |
| `$=` | Value ends with |
| `*=` | Value contains substring |
| `~=` | Whitespace-separated list includes value |
| `\|=` | Equals value or starts with `value-` |
| `[]` | Attribute is present (ignore value) |

### Supported pseudo-classes

Only the following pseudo-classes are accepted by the sightmap selector engine.
All others produce a parse error at `sightmap validate` time.

| Pseudo-class | Notes |
|---|---|
| `:not(sel)` | Negation — element must NOT match `sel` |
| `:is(sel, ...)` | Matches if element matches any of the alternatives |
| `:where(sel, ...)` | Same as `:is()` (zero specificity; treated identically here) |
| `:has(sel)` | Matches if a descendant satisfies `sel` |

**Unsupported** (produce a parse error): `:first-child`, `:last-child`,
`:first-of-type`, `:last-of-type`, `:nth-child()`, `:nth-of-type()`, `:hover`,
`:focus`, and all other dynamic or positional pseudo-classes.

**Workaround for identical siblings**: when two sibling elements share the same
class with no distinguishing attribute, merge them into a single component whose
selector matches both (e.g. one `CardActionButton` for Edit and Remove), or add
a `data-*` attribute to the source elements to make them selectable individually.

## Bounds

Viewport bounding box in pixels.

| Field | Type | JSON key |
|---|---|---|
| X | int | `x` |
| Y | int | `y` |
| Width | int | `width` |
| Height | int | `height` |

## ComponentNode

| Field | Type | JSON key | Phase | Description |
|---|---|---|---|---|
| Id | string | `id` | pre-merge | Globally unique node ID. Frame-prefixed for sub-frames (`"1_5"`). |
| Role | string | `role` | post-merge | WAI-ARIA role. `"StaticText"` for text nodes, `"none"` for ignored. |
| Name | string | `name` | post-merge | Computed accessible name (NOT raw `textContent`). |
| Text | string | `text` | post-merge | Rendered text content (`innerText`, `textContent` fallback), normalized to a single clean shape (whitespace runs collapsed, ends trimmed). Present for role-less nodes that have no accessible name; the fallback source for `extract: text`. Omitted when empty. |
| Value | string | `value` | post-merge | Current value for form controls. |
| Properties | map[string]string | `properties` | post-merge | Additional A11Y properties (`aria-*` etc.). |
| Element | \*Element | `element` | pre-merge | Observed element identity (tag/id/classes/attrs). Nil for virtual nodes. Matched against `SelectorPart` patterns. |
| Bounds | \*Bounds | `bounds` | pre-merge | Viewport bounding box. |
| IsVisible | bool | `isVisible` | pre-merge | Effective visibility, computed in-browser via `Element.checkVisibility` — false when the element or any ancestor is hidden (`display:none`, `visibility:hidden`, `opacity:0`, `content-visibility`). |
| IsInteractive | bool | `isInteractive` | pre-merge | Element is actionable per probe heuristics. |
| InViewport | bool | `inViewport` | pre-merge | Bounds intersect the viewport. |
| IsIgnored | bool | `isIgnored` | post-merge | A11Y tree marks this node as ignored. |
| NthChild | int | `nthChild` | pre-merge | 1-based position among parent's children. |
| Children | []\*ComponentNode | `children` | pre-merge | Direct children in document order. |

`omitempty` applies to: `Properties`, `Element`, `Bounds`, `Children`.

### Element

The observed identity of a node's underlying element — the concrete facts a live
DOM (or native) element presents. It is the *subject* a `SelectorPart` pattern is
matched against, and carries no matching operators or pseudo-classes
(`AttrOps`/`Not`/`Is`/`Has`) — those live only on the pattern side.

| Field | Type | JSON key |
|---|---|---|
| Tag | string | `tag` |
| Id | string | `id` |
| Classes | []string | `classes` |
| Attrs | map[string]string | `attrs` |
Boolean fields and `NthChild` are always present (zero value is meaningful).

### Phase notes

- **pre-merge**: populated by `probe.js` before A11Y data is available. Present
  even in raw probe output.
- **post-merge**: populated by `extract.BuildTree` after merging the
  `Accessibility.getFullAXTree` response. Empty string / false / nil in raw
  probe output.

## Example

```json
{
  "id": "42",
  "role": "button",
  "name": "Add to cart",
  "value": "",
  "properties": { "aria-pressed": "false" },
  "element": {
    "tag": "button",
    "id": "add-btn",
    "classes": ["primary", "action"],
    "attrs": { "data-testid": "add-to-cart" }
  },
  "bounds": { "x": 10, "y": 20, "width": 120, "height": 40 },
  "isVisible": true,
  "isInteractive": true,
  "inViewport": true,
  "isIgnored": false,
  "nthChild": 3,
  "children": [
    {
      "id": "43",
      "role": "StaticText",
      "name": "Add to cart",
      "value": "",
      "isVisible": false,
      "isInteractive": false,
      "inViewport": false,
      "isIgnored": false,
      "nthChild": 1
    }
  ]
}
```
