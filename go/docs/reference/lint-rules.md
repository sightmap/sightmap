# Lint rules

Part of the [Sightmap Authoring Reference](../reference.md).
See also: [Component model](component-model.md) · [Tool reference](tools.md) · [Quality checklist](quality-checklist.md)

---

Lint runs statically against `.sightmap/` YAML; no browser required. Always run with `--warn-only`.

---

## `child-repeats-parent`

**Principle:** Child selectors are automatically scoped under the matched parent's
DOM subtree. A child selector that re-states the parent's class produces a
doubled ancestor path after flattening (e.g. `.dashboard .dashboard .card`) and
will always match 0 nodes.

```yaml
# TRIGGERS — parent is '.dashboard', child restates it
- name: Dashboard
  selector: ".dashboard"
  children:
    - name: FreelancerCard
      selector: ".dashboard .freelancer-card"  # wrong: auto-scoped to .dashboard already

# FIX — drop the parent prefix in child selectors
- name: Dashboard
  selector: ".dashboard"
  children:
    - name: FreelancerCard
      selector: ".freelancer-card"  # correct: scoped automatically
```

**Why it happens:** Agents naturally author fully-qualified selectors. The
sightmap loader prepends the parent's selector before running the child,
so `.dashboard .freelancer-card` becomes `.dashboard .dashboard .freelancer-card`
internally — a selector that never matches.

**Fix:** Drop the parent prefix from child selectors. The engine scopes child
evaluation to each matched parent subtree; only the local, discriminating part
is needed.

---

## `broad-tag-selector`

**Principle:** A bare HTML tag at global scope matches too broadly.

```yaml
# TRIGGERS — bare 'header' at global scope
- name: SiteHeader
  selector: "header"

# DOES NOT TRIGGER — child component; parent provides scope
- name: NavFooter
  selector: "#footer-static"
  children:
    - name: FooterLink
      selector: "a"
```

**Fix:** Add a more specific anchor (`data-testid`, `aria-label`, structural context). If the tag genuinely uniquely identifies the component (e.g. a single `<main>` per page), accept the warning with a memory note explaining the intent.

---

## `deep-nesting`

**Principle:** Selectors with 4+ descendant levels are fragile and over-specified.

```yaml
# TRIGGERS — 4+ levels
selector: "header nav[aria-label='Main'] > ul > li > a"

# FIX — break into parent + child
- name: MainNav
  selector: "nav[aria-label='Main']"
  children:
    - name: MainNavLink
      selector: "a"
```

**Fix:** Introduce intermediate parent components and use simple child selectors instead of long chains.

---

## `id-hash-selector`

**Principle:** `#id` selectors using auto-generated IDs are brittle.

```yaml
# TRIGGERS — likely auto-generated
selector: "#sprig-feedback-container"
selector: "#modal_12849"
```

**Heuristic:** IDs containing numbers, hashes, or underscores after a word boundary are flagged.

**Fix:** Use `data-testid` or a semantic attribute. If the ID is a stable framework-defined hook (e.g. `#__next`), accept the warning with a memory note.

---

## `multi-instance-no-property`

**Principle:** A component whose selector is not uniqueness-anchored and has no properties produces identical annotations for every instance, making instances indistinguishable in event logs.

**Uniqueness-anchored** means: the selector contains `#id` or `[data-testid=...]` (exact-match attribute value).

**Broad selector patterns** (triggers if not uniqueness-anchored):
- Bare generic tags: `button`, `input`, `a`, `div`, `span`
- Generic tags with utility classes: `button.btn-danger`, `a.link-primary`
- Generic data-component attributes: `[data-component="product-card"]`

**Suppressions:**
- **Singleton names:** Components with names containing "Header", "Footer", "Nav", "Site", "Page", "Layout", "Modal", or "Dialog" are exempt (expected to have 1 instance).
- **Child components:** Components with a `ParentChain` (scoped within another component) are exempt.
- **Has properties:** Components with at least one `properties:` entry are exempt.
- **Unstable selectors:** Components marked `stability: unstable` are exempt (accepted T2 residual by design).

```yaml
# TRIGGERS — broad selector, no properties, not a singleton
- name: DeleteButton
  selector: 'button.btn-danger'

# TRIGGERS — data-component without properties
- name: ProductCard
  selector: '[data-component="product-card"]'

# DOES NOT TRIGGER — has properties to differentiate instances
- name: ProductCard
  selector: '[data-component="product-card"]'
  properties:
    - name: sku
      extract: attr:data-sku
    - name: label
      extract: text:h3

# DOES NOT TRIGGER — unique selector (data-testid)
- name: SubmitButton
  selector: 'button[data-testid="submit-form"]'

# DOES NOT TRIGGER — unique selector (#id)
- name: MainNav
  selector: '#main-navigation'

# DOES NOT TRIGGER — singleton name pattern
- name: SiteHeader
  selector: 'header[role="banner"]'

# DOES NOT TRIGGER — marked as unstable (category D, no stable selectors available)
- name: TemplateCardButton
  selector: '.Card-module__root___-zUI button'
  stability: unstable
  memory:
    - "Category D: CSS module class names only, no stable attributes available"
```

**Fix:** Add a `properties:` entry that extracts unique data from each instance. If the selector can be narrowed to unique attributes, update the selector instead. For child components, nest them under their parent component definition. For selector-unstable components (category D), mark as `stability: unstable` and document in `memory:`.
