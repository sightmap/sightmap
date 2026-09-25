# Component model

Part of the [Sightmap Authoring Reference](../reference.md).
See also: [Coverage model](coverage-model.md) · [Lint rules](lint-rules.md) · [Quality checklist](quality-checklist.md)

---

## Three component types

| Type | Defined in | Selector evaluation scope |
|------|-----------|--------------------------|
| Global | `components.yaml` | Document root, every page |
| View-scoped | `views/*.yaml` top-level | Document root, pages matching the view's `route` |
| Child | `children:` of any component | Parent's matched DOM subtree only |

## Rule 1 — Global uniqueness

Global components must have selectors that are unambiguous across the entire application. Their selectors run against `document`, every page. A selector that accidentally matches 40 elements across the app is worse than no selector.

```yaml
# BAD: 'header' matches on every page, possibly multiple times
- name: SiteHeader
  selector: "header"

# GOOD: uniqueness-anchored
- name: SiteHeader
  selector: '[data-testid="site-header"]'
```

## Rule 2 — Child scoping

Child selectors only need to discriminate within their parent's DOM subtree. The engine scopes evaluation to each matched parent element before running the child selector. This means children can use selectors that would be far too broad globally.

```yaml
- name: NavFooter
  selector: "#footer-static"
  children:
    - name: FooterLink
      selector: "a"         # Fine: only 'a' tags inside #footer-static
      properties:
        - name: label
          extract: text
    - name: FooterSocialLink
      selector: 'a[aria-label]'
      properties:
        - name: platform
          extract: attr:aria-label
```

## Rule 3 — Full-chain matching

A component is never matched in isolation. The engine narrows scope through each ancestor level before evaluating the selector. A child component is only reachable if its parent matched first.

## Properties: type vs. instance discrimination

**Type discrimination** — the selector identifies "this is a `ProductCard`, not a `NavLink`." Established by the selector itself, ideally via a stable `data-testid` or `data-component` attribute.

**Instance discrimination** — "this `ProductCard` shows the Garden Hose 50ft." Established by `properties:` extraction.

Both are needed for full event attribution. A component with no `properties:` produces identical annotations for all instances — every click on a `ProductCard` looks the same in the event log.

```yaml
- name: ProductCard
  selector: '[data-testid="product-pod"]'
  properties:
    - name: label
      extract: text
    - name: sku
      extract: attr=data-sku
```

### Extraction modes

Resolution runs over the component tree (SEP-0010), with no live DOM. To surface
a value from a sub-element, promote that sub-element to a declared `children:`
component and reference it via `PATH.prop` or `exists:PATH`.

| `extract:` | Result |
|------------|--------|
| `text` | the matched node's accessible text |
| `attr=NAME` | the value of attribute `NAME` carried by the node; omitted if not carried (e.g. `attr=aria-label`) |
| `PATH.prop` | the value extracted for property `prop` of the descendant component addressed by the dotted `PATH` |
| `exists:PATH` | `"true"` if a descendant component named by `PATH` matches; else the property is omitted (boolean state flag) |

## Selector quality hierarchy

Prefer the highest available option.

1. `[data-testid="exact-value"]` or `#stable-id` — uniqueness-anchored, ideal
2. `[data-component^="prefix"]` or `[aria-label="label"]` — stable semantic attributes
3. Tag + structural context — `main button[aria-expanded]`, `nav a[href^="/"]`
4. Class selectors — `.classname`, `[class*="partial"]`, `[class^="prefix-"]` — prefer `.classname` for exact matches when possible
5. Bare tag — only acceptable as a child in a narrow parent scope

**Combinators & pseudo-classes.** Descendant (` `) and direct-child (`>`)
combinators are supported; sibling combinators (`+`, `~`) are not.

**Supported pseudo-classes** (only these four):

| Pseudo-class | Notes |
|---|---|
| `:not(sel)` | Negation — element must NOT match `sel` |
| `:is(sel, ...)` | Matches if element matches any of the alternatives |
| `:where(sel, ...)` | Same as `:is()` (zero specificity; treated identically) |
| `:has(sel)` | Matches if a descendant satisfies `sel` |

`:has()` is relational — it scopes a component by something it *contains*, which
is the way to target a container that has no unique attribute of its own:

```yaml
# the form-group row that holds a checkbox (not the sibling radio row)
selector: '[data-testid="form-group"]:has(input[type="checkbox"])'
# combinators work inside :has() too — :has(> x) is a direct child
```

**Unsupported** (produce a parse error at `sightmap validate` time):
`:first-child`, `:last-child`, `:first-of-type`, `:last-of-type`,
`:nth-child()`, `:nth-of-type()`, `:hover`, `:focus`, and all other
positional or dynamic pseudo-classes.

**Workaround for identical siblings.** When two sibling elements share the same
class with no distinguishing attribute, merge them into a single component whose
selector matches both (e.g. one `CardActionButton` for Edit and Remove), or add
a `data-*` attribute to the source elements to make them individually selectable.

These behave identically in the offline matcher (`validate` / `snapshot` /
`coverage` / `sel-check`) and live (`sel-probe`). Always verify a new selector
with `sel-probe` or `sel-check` before writing it.

## Naming conventions

- **Name types, not instances:** `ProductCard` not `GardenHoseCard`
- **Name what the selector actually matches:** if the selector matches a kebab menu button, name it `ProductCardActions`, not `ProductCard`
- `*Content`, `*Panel`, `*Section`, `*Container` — selector anchors to the container element, not a child
- `*Button`, `*Link`, `*Input`, `*Menu` — selector matches an interactive leaf element
- `*List`, `*Grid`, `*Carousel` — repeating container

## The `stability:` field

Components and views support an optional `stability:` field indicating selector certainty or view completeness.

**Component stability values:**

| Value | Meaning | Tooling action |
|-------|---------|----------------|
| *(omitted)* | Stable — selector is verified and cross-page safe | None |
| `uncertain` | Selector works locally but hasn't been verified across all pages that include it. Needs `sel-probe --all` cross-check before promoting to global. | Flag in future validation output |
| `unstable` | Selector is known to be fragile (dynamic IDs, volatile CSS, etc.). Permanent T2 residual by design. | Suppresses `multi-instance-no-property` lint warning |

**View stability values:**

| Value | Meaning |
|-------|----------|
| *(omitted)* or `active` | Fully mapped |
| `stub` | Visited but deliberately not fleshed out (different stack, deferred, out-of-scope) |
| `deferred` | In-stack page, needs a view, not yet prioritized |

**Usage examples:**

```yaml
# Component with unstable selector (category D from T2 triage)
- name: AppHomeTemplateCardButton
  selector: '.Card-module__root___-zUI button'
  stability: unstable
  memory:
    - "Selector uses CSS module class names — no stable data-testid or aria-label available"

# Component pending cross-page verification
- name: DateFilterModal
  selector: '[data-testid="sheet"]'
  stability: uncertain
  memory:
    - "Works on ProductListPage; needs verification on SearchPage and CategoryPage"

# Stub view for out-of-scope page
views:
  - name: LegacyCMSPage
    route: /legacy/**
    stability: stub
    components: []
```

**When to use `stability: unstable`:**

Mark a component as `unstable` when:
- The element is visible and interactive
- No stable selector exists (CSS module classes, auto-generated IDs, dynamic attributes)
- You've documented what was tried in `memory:`
- You accept this as a permanent T2 residual

Marking `stability: unstable` suppresses the `multi-instance-no-property` lint warning for that component.
