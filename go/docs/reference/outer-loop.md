# The outer loop

Part of the [Sightmap Authoring Reference](../reference.md).
See also: [Tool reference](tools.md) · [Quality checklist](quality-checklist.md)

---

```
browser start
    └─ iterate URL              ← primary loop: navigate + snap + coverage
           ├─ fix YAML
           ├─ validate
           └─ iterate again
                 └─ T3 = 0 ✓
                       └─ quality self-review
                             ├─ T2 triage (--trace)
                             ├─ properties completeness
                             └─ zero-match investigation
                                   └─ multi-coverage     ← cross-page promotion
                                         └─ report       ← corpus health
                                               └─ snapshot --all
```

## Phase 0 — Session startup

```bash
cd sites/SITE.com/
sightmap browser start
sightmap browser status      # should show: ● running port=7892
```

The sightmap HTTP server starts automatically on port 7891 and hot-reloads on YAML changes. The overlay extension is embedded in the binary; no `--extensions` flag is needed.

## Phase 1 — Per-page iteration

`iterate` is the primary edit-verify loop. It navigates, snaps, and prints coverage + T2/T3 trace in one step.

```bash
sightmap iterate 'https://example.com/products'
```

Example output:

```
[View: ProductListPage "https://example.com/products"]
[Coverage] (visible only)
87 interactive · 21 direct T1 (24%) · 61 scoped T2 (70%) · 5 orphaned T3 ✗

Unlabeled clusters:
  5× button (no text)
       inside: div[data-testid="product-pod"]
       → [data-testid="product-pod"] button

Largest T2 scopes:
   47× [ProductGrid]
    6× [SiteFooter]
    3× [SiteHeader]
```

Edit `.sightmap/*.yaml`, then run `iterate` again. Repeat until T3 = 0.

When the output shows `T3 ✗`, the `Unlabeled clusters` section shows selector hints. Use `sel-probe` to validate a candidate before writing YAML.

```bash
sightmap sel-probe '[data-testid="product-pod"] button'
```

## Phase 2 — Quality self-review (after T3 = 0)

1. Re-run with `--trace` to see T2 scope detail:
   ```bash
   sightmap iterate --trace 'https://example.com/products'
   ```
2. For each T2 scope with > 1 child: run `sel-probe` on candidate selectors. If nothing stable exists, classify and document (A/B/C/D) in `memory:`.
3. Check every T1 interactive leaf has at least one property **in its ancestry chain** (either on the leaf itself or on a parent component).
4. Investigate all `[Warnings]` entries in the snap (zero-match components).

See the [Quality checklist](quality-checklist.md) for the full review pass.

## Phase 3 — Cross-page promotion

```bash
sightmap multi-coverage
```

Shows components appearing on 2+ pages. Promotion rule of thumb:

- **3+ pages, same selector:** always move to `components.yaml`
- **2 pages, same selector:** usually promote
- **2 pages, different selectors:** keep view-scoped

## Phase 4 — Validation and health check

```bash
sightmap validate            # structural correctness; exits non-zero on error
sightmap lint --warn-only    # style checks; exits 0, advisory only

sightmap report              # per-view T1/T2/T3 table + T2 quality analysis
sightmap snapshot --all      # refresh a snap for every view URL in views/*.yaml (requires browser)
```
