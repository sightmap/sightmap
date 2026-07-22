# Quality checklist

Part of the [Sightmap Authoring Reference](../reference.md).
See also: [Coverage model](coverage-model.md) · [The outer loop](outer-loop.md) · [Lint rules](lint-rules.md)

---

Run after reaching T3 = 0. Items marked *(auto)* are surfaced by tooling; *(manual)* require inspection.

## 1. T2 triage *(auto via `--trace`)*

Every T2 scope with > 1 interactive child is classified A/B/C/D and documented in `memory:`. Single-child T2 scopes are acceptable without notes.

```bash
sightmap iterate --trace 'https://example.com/products'
# or
sightmap coverage --trace product-list.snap
```

## 2. Properties completeness *(manual)*

Every T1 link and button has at least one extracted property. Scan the snap for bare `[ComponentName]` annotations on interactive leaf nodes — these are components without properties where every instance looks identical.

```bash
grep '\[ProductCard\]' product-list.snap     # bare annotation = no properties resolved
grep '\[ProductCard label=' product-list.snap # annotated instance = good
```

## 3. Multi-instance differentiation *(auto via lint go-d4e6)*

No component with N > 1 instances on any page has zero properties. The `multi-instance-no-property` lint rule will flag these automatically once implemented; until then, scan the snap manually for repeated bare component names.

## 4. Zero-match investigation *(auto via `[Warnings]`)*

Every entry in the `[Warnings]` section of the snap is explained:
- **Below-fold or conditional content** (carousel slide 2, modal, tooltip): add a `memory:` note saying so
- **Selector is wrong:** fix it

Zero-match components silently provide no coverage. They are not errors but they are waste.

## 5. Name/selector alignment *(manual)*

Verify naming conventions are respected:
- `*Card`, `*Panel`, `*Section`, `*Container`, `*Row` → selector anchors to the container element
- `*Button`, `*Link`, `*Input`, `*Menu` → selector matches an interactive leaf

A `ProductCard` selector that matches a button inside the card will scope child components incorrectly and produce confusing T2 scopes.

## 6. Duplicate selectors *(manual)*

No two components in the same evaluation context (same view, or global) share an identical selector. Duplicates cause the engine to match both and produce ambiguous annotations.

```bash
sightmap search --field selector 'data-testid="product-pod"'
```

## 7. Selector stability *(manual)*

No component uses volatile CSS module classes as its primary anchor. These are recognizable as:

- Class names with hash suffixes: `._Pod_3xK9a`
- Class names with encoded paths: `.ProductPod__container--2HkLm`

These change on every build. Document as category D in `memory:` and accept the T2 residual.
