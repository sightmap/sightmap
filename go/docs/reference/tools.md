# Tool reference

Part of the [Sightmap Authoring Reference](../reference.md).
See also: [The outer loop](outer-loop.md) · [Lint rules](lint-rules.md)

---

## `sightmap atlas find QUERY` / `list` / `add SLUG`

**Solves:** Finding out whether the site you are about to map has already been mapped, and starting from that corpus instead of authoring one from scratch. `find` searches the community atlas (`sightmap.org/atlas`), `list` browses it, `add` installs one entry into `--target` (default `.sightmap/`).

**Start with the domain.** An agent about to automate a site has a URL, not a slug. An exact domain match ranks first, and every hit prints the command that installs it:

```bash
sightmap atlas find squareup.com
square-pos  Square POS
  Point-of-sale checkout, catalog, and order history.
  squareup.com, app.squareup.com · payments, commerce · 12 views, 48 components, 23 requests · verified 2026-07-14
  sightmap atlas add square-pos

1 match.
```

The counts are the ones the gallery card shows, read from the same `index.json`.

Then install what it printed:

```bash
sightmap atlas add square-pos
  wrote  .sightmap/components.yaml
  wrote  .sightmap/config.yaml
  wrote  .sightmap/views/checkout.yaml

Installed square-pos: 3 files → .sightmap. Next:
  sightmap validate
```

Run any verb with `--help` for its flags. The ones that matter to an agent: `--json` on `find`/`list`, `--category` to narrow, `--target` on `add`, and `--source`/`--index` to point at a private store.

**Both defaults point at `sightmap.org/atlas`,** the index and the archives. A takedown rebuilds the gallery, so an entry the atlas removes stops being findable *and* stops being installable. Reading either one out of the git repo would leave half of that unenforced.

**Install from your own corpus store** by pointing `--source` at a URL template that takes `{slug}`; `--index` does the same for a private catalog. Both stay under the HTTPS-only transport policy.

```bash
sightmap atlas add toast-pos --source https://internal.corp/{slug}.tar.gz
```

**`sightmap atlas validate FILE`** is the publisher's check, not the reader's: duplicate slugs, uninstallable slugs, and control characters in display text, all reported in one run. `find` escapes that text as it parses, so this is the only place those problems surface.

**A search that finds nothing exits 0.** It answers the question it was asked. An `add` with an unpublished slug exits 1 and points at `find` and the atlas.

**Never merges.** A non-empty target is refused before anything is fetched, so the message is the same offline. There is no `--force`. Delete the directory yourself if you meant to replace it.

**`find`/`list` read the index; `add` does not.** `add` fetches one `.tar.gz` from a URL template, so an index outage or a schema bump cannot stop an install, and the index can grow fields without a CLI release. `find` and `list` cache the index at `~/.sightmap/atlas/index.json` for 24 hours.

**Untrusted by construction.** Fetches are HTTPS-only (loopback excepted) with the policy re-applied to every redirect hop. An archive is capped on the wire *and* decompressed, per file, and by member count; every member must be a regular file or directory under `.sightmap/`, with no absolute path, traversal, symlink, or control character. The corpus is loaded before the rename that installs it, so an entry that does not load is reported as an atlas defect with nothing written. Index-supplied text is escaped before it reaches your terminal.

**Gotcha:** `add` proves the corpus *loads*; it does not vet its content. Run `sightmap validate` (the command tells you to) and read the YAML. An atlas entry's `memory:`/`description:` text ends up in your agent's context.

---

## `sightmap browser start`

**Solves:** Launching a Chrome session with the sightmap HTTP server and overlay extension.

**Key flags:**

| Flag | Default | Purpose |
|------|---------|---------|
| `--port` | `7892` | Chrome DevTools Protocol port |

**Config:** `browser.port` in `.sightmap/config.yaml`.

The overlay extension is embedded in the binary and auto-extracted to `~/.sightmap/extension/` on first start or when the bundled version changes.

---

## `sightmap browser navigate URL`

**Solves:** Navigating the live Chrome session to a URL.

**Gotcha:** Takes a positional URL argument — NOT a `--url` flag.

```bash
# CORRECT
sightmap browser navigate 'https://example.com/products'

# WRONG — passes the literal string "--url https://..." as the URL
sightmap browser navigate --url 'https://example.com/products'
```

**Gotcha:** Prints the requested URL, not the final URL after redirects. To confirm the landed URL:

```bash
sightmap browser eval 'document.location.href'
```

---

## `sightmap browser eval 'JS'`

**Solves:** Running arbitrary JavaScript in the page context for debugging or probing.

```bash
sightmap browser eval 'document.querySelectorAll("[data-testid]").length'
sightmap browser eval 'document.title'
```

**Returns:** JSON-serializable values only. `document.querySelector(...)` returns an error; chain `.textContent`, `.getAttribute()`, etc.

**Use when:** Debugging page state or querying the DOM directly.

**Note:** If you need to probe elements that require focus state (e.g., search autocomplete dropdowns), this is currently the only workaround. Once persistent focus state in sel-probe ships, this workaround won't be necessary.

---

## Interaction by component query (the targeting DSL)

**Solves:** Clicking / filling / hovering a sightmap component reliably on
dynamic, re-rendering pages — without round-tripping for a probe ID.

`browser click`, `fill`, `hover`, and `scroll --component-id` accept **either** a
numeric probe ID (from `snapshot` output) **or** a component query. A query
resolves *atomically* — extract → match → act in a single pass — so no ephemeral
ID crosses a call boundary and nothing can go stale between resolve and act
(the failure mode of `click <id>` on React pages, where the tree re-renders and
reassigns IDs).

```bash
sightmap browser click 'FulfillmentTileButton[label=deliveryTile]'
sightmap browser click 'ProductCard[name^="Weber"]'
sightmap browser fill  'SearchInput' 'cordless drill'
sightmap browser click 'AddToCartButton#1'        # occurrence index (0-based)
```

The query is CSS-shaped but matches over **sightmap components and their
extracted properties**, not raw DOM:

| Form | Resolves to |
|------|-------------|
| `Name` | the component named `Name` |
| `Name[prop=val]` | filter on an extracted **property** (not a DOM attribute) |
| `Name[prop^=val]` / `Name[prop*=val]` | prefix / substring (exact `=`, prefix `^=`, substring `*=` — no suffix/word ops yet) |
| `Name[prop=val i]` | case-insensitive (trailing ` i`) |
| `Name[a=1][b^=x]` | multiple predicates on one component (all must hold) |
| `Ancestor Descendant` | whitespace = descendant; the **last** component is the target |
| `Name#N` | occurrence index (0-based) when several match — weak fallback |

**Notes & gotchas:**
- Predicates filter on **extracted properties**, so robust targeting depends on
  the corpus defining disambiguating properties (e.g. `FulfillmentTileButton.label`
  or a `match:`-derived `method`). Property work and reliable targeting reinforce
  each other — see [Properties](component-model.md#properties-type-vs-instance-discrimination).
- Zero matches → error; multiple matches → error that lists the candidates with
  their distinguishing properties (add a predicate, or `#N`).
- `--sightmap-dir` selects the corpus used to resolve the query.
- **Flag ordering:** put flags *before* the query — `click --tab T 'Query'`. A
  positional placed before the flags is silently ignored.
- The final act is still a coordinate dispatch, so an off-screen target can be
  missed; scroll it into view first if needed. Raw `--x N --y N`
  remains as a layout-fragile escape hatch.

---

## `sightmap snapshot`

**Solves:** Observe a live page — the full annotated ARIA tree with coverage stats, printed to stdout (or `--out FILE`). Pure read: `snapshot` never writes into the corpus and never novelty-gates.

**Key flags:**

| Flag | Purpose |
|------|---------|
| `--url URL` | Navigate before snapping |
| `--coverage` | Print `[View]` + `[Coverage]` + cluster traces only, suppressing the tree (the fast edit-loop view) |
| `--out FILE` | Write the annotated output to FILE instead of stdout (plain file write — no view set, no gate) |
| `--tree-out FILE` | Write raw tree JSON for offline coverage/multi-coverage |
| `--trace` | Include T3 cluster detail and T2 scope breakdown |
| `--include-hidden` | Include hidden/off-screen nodes in coverage stats (default: visible only) |
| `--wait N` | Seconds to wait after navigation (for async rendering) |

**Config:** `snapshot.wait`, `snapshot.trace`, `snapshot.include_hidden` in `.sightmap/config.yaml`.

`--coverage` is the primary edit-verify loop: navigate + coverage + T2/T3 trace in one command, tree suppressed. Edit `.sightmap/*.yaml`, re-run, repeat until T3 = 0.

```bash
sightmap snapshot --coverage --url 'https://example.com/products'   # edit loop
sightmap snapshot --url 'https://example.com/products'              # full annotated tree
sightmap snapshot --url 'https://example.com/products' --out page.snap --tree-out page.snap.tree.json
```

**Common mistake:** Snapping before async content renders (React Suspense, lazy routes). Use `--wait 2` or set `wait: 2` in `.sightmap/config.yaml`.

---

## `sightmap capture`

**Solves:** Persist a capture into the matched view's set. `capture` extracts the tree exactly as `snapshot` does, then appends it to `.sightmap/snapshots/{view}/{stamp}.snap` (+ a `.snap.tree.json` sibling), subject to the novelty gate. Requires a matching view — an unmapped page is an error (use `snapshot` to observe one).

**Key flags:**

| Flag | Purpose |
|------|---------|
| `--url URL` | Navigate before capturing |
| `--all` | Capture every representative URL declared in `views/*.yaml` (`url:` + `snapshots[].url`) |
| `--force` | Write the capture even if it adds no new component/slot vs the view set (skip the novelty gate) |
| `--json` | Also write an annotated JSON sibling next to each capture |
| `--include-hidden` | Include hidden/off-screen nodes in analysis |
| `--wait N` | Seconds to wait after navigation (for async rendering) |

**Config:** `snapshot.wait`, `snapshot.trace`, `snapshot.include_hidden` in `.sightmap/config.yaml`.

**Snapshot organization:** A **view** is a **set** of timestamped captures stored at `.sightmap/snapshots/{view}/{stamp}.snap` (`{stamp}` = UTC `YYYYMMDDTHHMMSSZ`). There is no per-view "state" axis — just re-`capture` the view in whatever configurations you want (different loads, a drawer open, a tab switched); the **novelty gate** keeps only captures that add a new component type or orphan slot and the **union** is what coverage scores. Capture each view a few times to cover lazy / rotating / interaction-gated sections. `--all` reads representative URLs from `views/*.yaml` (see *View URLs* below) and dogpiles every target into its view's set. A capture that adds nothing new is skipped; `--force`, or the first capture of a view, always writes.

**View URLs (canonical):** `capture --all`, `sel-probe --all`, and `report` source their URLs from `views/*.yaml`, not from a separate file. Each view declares a top-level `url:` (its primary page) and may list `snapshots:` entries, each with an optional `url:` to capture a variant (e.g. a category-landing page vs. a leaf PLP). A view with no `snapshots:` yields a single `base` snapshot from its `url:`.

```yaml
# views/plp.yaml
url: https://example.com/c/leaf-category      # → snapshots/plp/{stamp}.snap
snapshots:
  - notes: Leaf category with the product grid (uses the view url).
  - notes: Department landing page — no product grid.
    url: https://example.com/c/department      # → also snapshots/plp/{stamp}.snap
```

```bash
# Append a novelty-gated capture to .sightmap/snapshots/{view}/
sightmap capture --url 'https://example.com/'

# Refresh every view URL declared in views/*.yaml
sightmap capture --all
```

---

## `sightmap coverage`

**Solves:** Offline T1/T2/T3 re-check against saved `.snap.tree.json` files. No browser needed.

```bash
sightmap coverage --trace home.snap
sightmap coverage home.snap product-list.snap checkout.snap
```

Automatically loads the corresponding `.snap.tree.json` for each `.snap` argument.

**Union-aware over a view's set.** `coverage` groups captures by view and reports per-DOM T1/T2/T3 for each capture, but judges component **presence** across the whole set: a `[Warnings]` line fires only when a component matched `0 of N snaps` (union-dead), while components present in just *some* captures appear under `[Presence]` as `matched in K of N snaps`. So a carousel that drops out of a single reduced load is **not** flagged dead.

```
[Warnings]
  HomePage: EarnedMediaBar — 0 matches (0 of 8 snaps)

[Presence] (matched in a subset of the view's snapshots)
  HomePage: DigitalEndcap — matched in 1 of 8 snaps (last 2026-06-08 00:00Z)
```

`coverage`, `report`, and `multi-coverage` are the three set-aware readers — same “operate over the union of the view's set” rule, specialized per job: `coverage` → per-component dead/partial; `report` → aggregate health row; `multi-coverage` → cross-page presence matrix. `report`'s `✗` is wired to match `coverage`'s union fail.

**Annotation gaps (advisory).** Because T1/T2/T3 only score *interactive* nodes, a non-interactive content node whose accessible name carries real information (e.g. a banner `image "UP TO $800 OFF…"`) is structurally invisible to coverage. `coverage` prints an extra advisory section listing such nodes — a non-trivial accessible name, not matched by a component, and with no matched ancestor (zero component context):

```
[Annotation gaps] (named content with no component context)
  HomePage: image "UP TO $800 OFF SELECT…" inside div[data-component^="hero"] — 1 of 8 snaps
```

It's a precision-biased nudge, **not** a gate (never changes the exit status): a named node *inside* a matched component is not flagged, and once a component matches the node it becomes T1 and drops off. Fix by adding a component that captures the content (so its name becomes an extracted property).

**Common mistake:** Running with `--include-hidden` unintentionally — invisible nodes inflate T3 counts and produce false failures. Visible-only mode is the default.

---

## `sightmap report`

**Solves:** "Are we done?" — per-view coverage health table aggregated over each view's whole capture **set**. Shows T2 quality analysis across views.

```
sightmap report

sightmap report · homedepot.com · 2026-06-08
──────────────────────────────────────────────────────────────────
 View   Route    Snaps   T1     T2    T3   Largest T2 scope
──────────────────────────────────────────────────────────────────
 home   /          3     78%    22%    3 ✗  [GlobalNav] (7)
 plp    /b/**      2     82%    17%    0 ✓  [FacetFilter] (8)
 pdp    /p/**      1     83%    17%    0 ✓  [GlobalNav] (7)
──────────────────────────────────────────────────────────────────
 2/3 views ✓  ·  1 ✗  ·  avg T1 80%  ·  avg T2 18%
```

**How a view's set rolls up to one row:**

| Column | Aggregation over the view's N captures |
|--------|----------------------------------------|
| `Snaps` | set size N |
| `T1` / `T2` | `Σt1` / `Σt2` over `Σtotal` (a weighted average across every interactive node captured for the view) |
| `T3` | **max** orphan count across the set; `✗` iff `> 0` |
| `Largest T2 scope` | max single T2 cluster seen across the set |

The **max-T3 gate is what makes `report` agree with `coverage`**: both fail iff *some* capture in the set has an orphan. (Previously `report` showed only the latest capture, so a reduced final load could read `0 ✓` while `coverage` failed the set.) Worked example: `home` above has captures with T3 = 3, 0, 0 → row shows `3 ✗`; T1 = `(410+270+250)/(535+340+320)` = 78%.

The `⚠` flag on T2 scopes indicates multi-child scopes that warrant investigation.

**Requires:** saved `.snap.tree.json` files (run `capture --all` first). URLs come from each view's `url:` / `snapshots[].url` in `views/*.yaml` (see *View URLs* under `snapshot`).

---

## `sightmap gap`

**Solves:** Finding interactive nodes on the live page with no sightmap component context (T3 orphans). Operates on the live DOM, not a snap file.

```bash
sightmap gap

Unlabeled interactive nodes:
  3× button (no text)
       inside: div[data-testid="product-pod"]
       → [data-testid="product-pod"] button[aria-label]
  1× a  href="/checkout"
       inside: header[data-testid="site-header"]
       → [data-testid="site-header"] a[href="/checkout"]
```

Output includes selector hints to seed `sel-probe` investigation.

**Visible-only is the default.** Use `--include-hidden` only when you specifically need to audit hidden-but-interactive nodes.

**Planned:** `gap --scope COMPONENT_NAME` — show T2 nodes within a specific component's subtree.

---

## `sightmap sel-probe 'selector'`

**Solves:** Validating a CSS selector before writing YAML. Shows match count, per-match attributes, DOM parent chain, and nearest known sightmap component ancestor.

```bash
sightmap sel-probe '[data-testid="add-to-cart"]'

selector: [data-testid="add-to-cart"]
matches: 4
  [1] button  data-testid=add-to-cart  aria-label="Add Garden Hose 50ft to cart"
      parents: div[data-testid="product-pod"] > main
      ★ nearest component: ProductCard (div[data-testid="product-pod"])
  [2] button  data-testid=add-to-cart  aria-label="Add Sprinkler Head to cart"
      parents: div[data-testid="product-pod"] > main
      ★ nearest component: ProductCard (div[data-testid="product-pod"])
  ...
```

**Key flags:**

| Flag | Purpose |
|------|---------|
| `--all` | Run against every view URL in `views/*.yaml` (`url:` + `snapshots[].url`) |
| `--tab ID` | Target a specific tab (required when several content tabs are open) |

**Gotcha:** `sel-probe` currently loses browser focus state. Elements visible only while an input is focused (e.g. search autocomplete suggestions) cannot be probed this way. Workaround: use `browser eval 'document.querySelectorAll(...).length'` with the focus state already established.

**Planned:** `--scope ANCESTOR_SELECTOR` — verify child selector matches only within a specific ancestor.

---

## `sightmap multi-coverage`

**Solves:** Cross-page coverage matrix. Shows which components appear across multiple pages and surfaces global-promotion candidates.

```bash
sightmap multi-coverage
```

One column **per view**, not per capture file. A view's many captures are folded into a single column whose cell is the **max** matched count across the set (union-honest “renders *up to* K of these”; `-` = matched in no capture). The header reads `view·N` when the view carries N>1 captures.

```
Component         home·3   plp·2   pdp
────────────────────────────────────────
AddToCartButton        -       -     1
DigitalEndcap          4       -     -      ← max rescues a carousel that rendered
FacetFilter            -       8     -        in only 1 of home's 3 captures
ProductPod            12      36     6
PromoBanner            2       -     -
SiteHeader             1       1     1
SortDropdown           -       1     -

Global candidates (appear in 2+ views, not yet in global components):
  ProductPod    home(12) plp(36) pdp(6)   → add to components.yaml
```

**Candidates count views, not files.** A component is a promotion candidate when it appears in 2+ *views* and isn't already global. (Previously the rule was “2+ files,” so a single multi-capture view falsely flagged its own view-local components — e.g. `FacetFilter`/`PromoBanner` above, each in just one view, are correctly *not* candidates.)

**Requires:** `.snap.tree.json` files for each page, written by `capture` (or `snapshot --tree-out`). Run `capture --all` first if stale.

**When to use:** After completing per-page iteration, before declaring the corpus done.

---

## `sightmap validate`

**Solves:** Structural correctness of `.sightmap/` YAML. Exits non-zero on any error.

```bash
sightmap validate
✓ no validation errors
```

Checks: valid YAML, required fields present, no unknown keys, selector syntax parseable, route patterns valid.

Run after every YAML edit and before every commit.

---

## `sightmap lint`

**Solves:** Style and quality checks. Always use `--warn-only` (exits 0, advisory only).

```bash
sightmap lint --warn-only
```

No browser needed — runs statically against `.sightmap/` YAML. See [Lint rules](lint-rules.md) for details.

---

## `sightmap stats`

**Solves:** "What's actually in this corpus?" — corpus-wide totals plus a per-view component/request table, counted against the **loaded** model (`$ref`s expanded, hierarchies flattened).

```
sightmap stats

sightmap stats · example.com · 2026-08-05
──────────────────────────────────
 Views       3
 Components  8
 Requests    3
 Properties  3
 Memory      2
──────────────────────────────────
 View  Route  Components  Requests
──────────────────────────────────
 home  /               4         0
 pdp   /p/**           3         1
 plp   /b/**           5         1
──────────────────────────────────
 3 views  ·  8 distinct components (per-view rows sum to 12)
```

**What each total counts:**

| Total | Counted over |
|-------|--------------|
| `Views` | view definitions |
| `Components` | distinct component **names** corpus-wide — the corpus vocabulary |
| `Requests` | global + view-scoped request definitions |
| `Properties` | `properties:` entries over distinct component **definitions** |
| `Memory` | `memory:` entries at file, view, component, and request level |

**Per-view rows do not have to sum to `Components`, in either direction** — a global `$ref`-reused by three views fills three rows but one slot in the total, while a global no view references fills a slot and no row. The summary line prints both numbers so the gap is legible.

The two rules differ on purpose: `Components` dedupes by name, but `Properties`/`Memory` sum over distinct definitions, because two views may legally define different components under the same local name (only *global* name collisions are rejected). A top-level `$ref` expands to a byte-identical copy and counts once; a `$ref` under a parent is scoped to that parent, so it is a separate extraction site and counts again.

**`--json` is a published contract.** One JSON object on stdout, no banner: `views`, `components`, `requests`, `properties`, `memory`, `per_view[{name, route, components, requests}]`. Consumed by CI outside this repo (the atlas index generator), so the field names never change. The same counts are available in-process as `sightmap.Corpus.Stats()` — prefer that over shelling out.

**Refuses a corpus it cannot count.** The loader drops an unresolved `$ref` or a component missing its `name`/`selector` and records an error, which would make the counts a silent under-report. `stats` therefore runs `validate`'s checks first and exits 1 on any error-severity finding (warnings are advisory). Under `--json` the failure is still one parseable object — `{"error": ..., "diagnostics": [...]}`, with `error` present only on failure and no counts.

**When to use:** Orientation on an unfamiliar corpus, and as the CI numbers that tell you whether a corpus grew or shrank.

---

## `sightmap suggest --exclude-known`

**Solves:** Discovering DOM elements with stable selectors not yet in the sightmap.

```bash
sightmap suggest --exclude-known
```

**Always use `--exclude-known`** to suppress components already in the corpus.

**When to use:** When starting a new page or when looking for candidates to close a T2 gap.

---

## `sightmap discover`

**Solves:** URL pattern discovery — crawls page links and classifies each pattern.

```
✓ mapped   — pattern matches a view route in the corpus
○ surveyed — pattern in survey.yaml but no view component
? unseen   — not in the corpus at all
```

**When to use:** At the start of a new site to understand the URL space before authoring.

---

## `sightmap search PATTERN`

**Solves:** Offline regex search across all `.sightmap/` YAML, with component hierarchy context.

```bash
sightmap search 'Card'
sightmap search --field selector 'data-testid="product'
sightmap search --field memory 'category D'
```

**Key flags:**

| Flag | Default | Purpose |
|------|---------|---------|
| `--field` | all fields | Restrict to `name`, `selector`, `description`, or `memory` |

---

## `sightmap inspect`

**Solves:** Raw DOM tree for selector authoring — shows every element with its attributes, without the sightmap layer. Use when you need to find an anchor for a new component and `sel-probe` already presupposes a selector.
