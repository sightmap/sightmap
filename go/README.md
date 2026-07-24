# sightmap

`sightmap` layers semantic component names onto a live page's component tree. A
YAML **corpus** (the "sightmap") names the interactive parts of a site, and the
binary runs that corpus against a real browser session via the Chrome DevTools
Protocol to produce two outputs:

- **Annotated snapshots** — an ARIA component tree where every interactive node
  carries a `[ComponentName prop="val"]` annotation drawn from the corpus.
  Structured context for agents.
- **Structured events** — click / navigate / change records attributed to their
  enclosing component, enabling replay, analysis, and training-data generation.

The corpus lives under a site's `.sightmap/` directory. Authoring it is an
edit-verify loop: snap the page, see which interactive nodes are still
unattributed, add components, re-snap, repeat until coverage is complete.

## Install

```bash
# Install globally via npm (no Go toolchain required):
npm install -g @sightmap/sightmap

# Or one-off:
npx @sightmap/sightmap snapshot ...

# Developers building from source:
go install github.com/sightmap/sightmap/go/cmd/sightmap@latest
```

The overlay browser extension is embedded in the binary and auto-extracted to
`~/.sightmap/extension/` on first `browser start` — no `--extensions` flag
needed.

---

## Quickstart: producing a sightmap

This walks the basic loop for an existing site that already has a `.sightmap/`
corpus (or where you're starting one).

### 1. Start a browser session

```bash
cd sites/SITE.com/          # a directory containing a .sightmap/ corpus
sightmap browser start      # launches Chrome + the sightmap HTTP server (port 7891)
sightmap browser status     # should show: ● running cdp=7892
```

`browser start` launches a Chrome session and a local server that
hot-reloads the corpus whenever you edit YAML.

### 2. Iterate on a page

`iterate` is the primary edit-verify loop — it navigates, snaps, and prints
coverage in one step:

```bash
sightmap iterate 'https://SITE.com/products'
```

```
[View: ProductListPage "https://SITE.com/products"]
[Coverage] (visible only)
87 interactive · 21 direct T1 (24%) · 61 scoped T2 (70%) · 5 orphaned T3 ✗

Unlabeled clusters:
  5× button (no text)
       inside: div[data-testid="product-pod"]
       → [data-testid="product-pod"] button
```

Coverage assigns every interactive node a tier:

| Tier | Meaning | Goal |
|------|---------|------|
| **T1** direct | node has its own `[ComponentName]` annotation | maximize |
| **T2** scoped | node is inside a named component but unnamed itself | acceptable |
| **T3** orphaned | no matched component ancestor at any depth | **must be 0** |

The `Unlabeled clusters` section shows the orphaned (T3) nodes with a suggested
selector (the `→` line). Verify a candidate selector before writing YAML:

```bash
sightmap sel-probe '[data-testid="product-pod"] button'
```

Then add a component to `.sightmap/views/*.yaml` (or `components.yaml` for a
global one), e.g.:

```yaml
- name: AddToCartButton
  selector: '[data-testid="product-pod"] button[data-testid="atc"]'
  properties:
    - name: label
      extract: attr=aria-label
```

Re-run `iterate` and repeat until `0 orphaned T3 ✓`.

### 3. Validate, snapshot, and check corpus health

```bash
sightmap validate            # structural YAML correctness (exits non-zero on error)
sightmap lint --warn-only    # advisory style checks
sightmap snapshot --all      # refresh a saved snap for every view URL
sightmap report              # per-view T1/T2/T3 health table
```

For the full authoring workflow — coverage tiers, the component model, property
extraction, cross-page promotion, and the quality checklist — see the
[authoring reference](docs/reference.md). The bundled
[sightmap-authoring skill](../skills/sightmap-authoring/SKILL.md) drives an agent through the
same loop.

> The canonical skills live at the repo root under [`../skills/`](../skills/).
> The copy under `skills/` here is generated (`go generate ./skills/...`) and
> embedded into the binary for `sightmap skills install` — edit the canonical
> copy, not this one.

---

## Documentation

| Doc | What it covers |
|-----|----------------|
| [docs/reference.md](docs/reference.md) | Authoring reference — coverage model, component model, the outer loop, full tool reference, lint rules, quality checklist |
| [../skills/sightmap-authoring/SKILL.md](../skills/sightmap-authoring/SKILL.md) | Authoring skill — building/maintaining a corpus (agent workflow) |
| [../skills/sightmap-browser/SKILL.md](../skills/sightmap-browser/SKILL.md) | Browser skill — driving a live session and interacting via a corpus |

---

## Library

This repo is also the shared Go library for the sightmap component model,
selector matching, and browser-side component extraction. Its goals:

1. **Canonical component model and sightmap rule matching.** Downstream
   consumers import this library rather than re-implementing matching.

2. **Single implementation for all consumers.** External sightmap tools
   (authoring CLI, analysis scripts, native mobile) use the same library so
   component semantics and rule matching cannot diverge across implementations.

3. **Tree-level matching, not DOM queries.** Matching operates against the
   component tree — each node carries a `SelectorPart` struct — rather than
   via `document.querySelectorAll`. This enables offline analysis of exported
   snapshots and native-mobile compatibility via synthetic selectors.

4. **Canonical probe.js.** The `probe/probe.js` in this repo is the canonical
   implementation. Downstream consumers embed it from here so bounds,
   visibility, and interactivity logic cannot diverge.

### Package map

| Package | Description |
|---|---|
| `comps/` | `ComponentNode`, `SelectorPart`, `Bounds` types and tree operations |
| `sel/` | CSS selector string parsing and single-node `SelectorPart` matching |
| `match/` | NFA sightmap rule matching against component trees |
| `extract/` | A11Y merge and tree-build logic (no browser dependency) |
| `browser/` | Thin `Page` interface and direct-CDP adapter |
| `probe/` | Canonical `cdp-probe.js` (embedded; downstream consumers import from here) |
| `conformance/` | Shared test fixtures (component tree JSON + sightmap YAML + expected matches) |

### Requirements

- Go 1.25.2+

### Testing

```
go test ./...
```
