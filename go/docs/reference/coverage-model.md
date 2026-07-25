# Coverage model

Part of the [Sightmap Authoring Reference](../reference.md).
See also: [Component model](component-model.md) · [Quality checklist](quality-checklist.md)

---

Every interactive node in the DOM receives one of three coverage tiers based on how it is resolved against the sightmap corpus.

## Tiers

**T1 — Direct match**

The interactive node itself is matched by a component selector.

```
click [ProductCard label="Garden Hose 50ft"]
```

We know *what* was clicked and *which instance* it was.

---

**T2 — Scoped (ancestor match)**

An ancestor is matched but the node itself is not. The node is inside a named component but has no name of its own.

```
click [ProductGrid] link "Garden Hose 50ft"
```

We know *where* in the page hierarchy the interaction occurred but not *what* the element represents.

---

**T3 — Orphaned**

No matched ancestor at any depth.

```
click link "Garden Hose 50ft"
```

No semantic context. The click is unattributable.

---

## Done signal

**T3 = 0 is necessary but not sufficient.** T2 quality matters too.

## T2 quality

**T2-tight (acceptable):** The T2 node is the only interactive child of its nearest T1 ancestor. The component is a simple wrapper; the element is unambiguous from its role + text alone.

**T2-loose (investigate):** N > 1 interactive children share the same T1 ancestor — they are indistinguishable in event attribution. A broad container needs child components.

The `--trace` flag on `snapshot` and `coverage` surfaces T2 scopes ranked by child count. Single-child T2 scopes are acceptable; multi-child T2 scopes are the authoring queue.

## T2 triage categories

Record in the component's `memory:` notes after investigation.

| Code | Category | Guidance |
|------|----------|----------|
| **A** | Third-party / injected | Analytics, live chat, consent widgets. Acceptable T2. Note the vendor. |
| **B** | Exhausted | Tried `data-testid`, `aria-label`, `data-component`, href patterns — nothing stable. Document what was tried. |
| **C** | Untried | Not yet investigated. Default state. Always run `sel-probe` before accepting as exhausted. |
| **D** | Selector-unstable | Visible and interactive but anchors only in volatile CSS module classes, auto-generated IDs, or similar. Document the instability. Mark with `stability: unstable` to suppress lint warnings. |

Example memory note:

```yaml
- name: ProductGrid
  selector: '[data-testid="product-grid"]'
  memory:
    - "T2-loose: 47 children. Investigated add-to-cart (data-testid stable → child added).
       Remaining buttons use CSS module classes (_3xK9a etc.) — category D.
       Wishlist button: no stable attribute — category B, tried aria-label/data-testid/href."

# Category D example with stability field
- name: TemplateCardButton
  selector: '.Card-module__root___-zUI button'
  stability: unstable
  memory:
    - "Category D: CSS module class names only, no stable attributes available"
```
