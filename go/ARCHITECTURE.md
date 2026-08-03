# Architecture

How the `go/` implementation is organized: the package layering, the small set
of high-level library entrypoints, and how the CLI commands map onto them.

> **Status.** This is the *target* architecture that the CLI-teardown work is
> converging on. Packages marked **(planned)** below do not exist yet — they are
> being extracted from `cmd/sightmap/` incrementally. When a planned package
> lands, drop its tag. The design itself is ratified; treat it as the contract
> new code is written against.

## Three tiers

The code is organized in three tiers, from most reusable to least:

- **Subsystems** — cohesive packages with a clear responsibility and a stable
  exported surface. This is the library that external callers (e.g. Subtext)
  build against.
- **Operations** — a *small, curated* set of high-level entrypoints that compose
  several subsystems into one useful action (observe a page, score coverage,
  append a capture). They live **inside** the relevant subsystem package. A few
  map 1:1 onto a CLI command; that is the exception, chosen deliberately, not a
  blanket rule.
- **CLI adapters** — the `cmd/sightmap/*.go` files. Each is a thin shell:
  parse flags → call a subsystem/operation → format output. Adapters are never
  imported as a library.

There is deliberately **no "commands-as-library" tier**. Most commands are pure
glue over a single subsystem call; giving each one a mirrored `RunX(opts)`
library twin would just duplicate the subsystem behind a CLI-flavored options
struct. Callers use the subsystems and the curated operations directly.

Package boundaries follow **dependency cohesion**, not the CLI menu or the docs
navigation. `coverage` is used by six commands and belongs to none of them.

## Packages

Foundation (offline, no browser dependency):

| Package | Responsibility |
|---|---|
| `comps` | The canonical component model: `ComponentNode`, `SelectorPart`, `Bounds`. The wire format between the probe, extraction, and matching. |
| `sel` | CSS-selector parsing + single-node matching against `comps.SelectorPart` (the sightmap selector subset). |
| `compquery` | The component-query DSL — addressing elements by sightmap identity (component name + extracted properties + descendant chain) rather than by ephemeral probe id. |
| `match` | The NFA multi-query matcher: one O(M) pass over a `comps` tree matching N corpus rules simultaneously. |
| `extract` | Post-probe processing: merge A11Y data into probe output, rebuild the parent/child tree. No browser dependency. |
| `sightmap` | Loads `.sightmap/` YAML into a compiled, queryable **`Corpus`** (renamed from `Session`). Stays browser-free so every offline tool can use it. Also owns site config (`config.yaml`) and view-URL enumeration. |
| `probe` | Embeds the canonical `cdp-probe.js` browser-side extractor. |
| `render` | Formats a `comps` tree (+ matches) into the annotated snapshot and the raw `inspect` tree. Presentation only. |
| `coverage` **(planned)** | Pure T1/T2/T3 coverage math over a tree + matches: parent map, orphan slots, annotation gaps, T2/T3 cluster traces. |
| `viewset` **(planned)** | On-disk capture sets: paths, discovery, stamps, the novelty gate, and the prune planner. Offline; built on `coverage`. |

Live (browser-dependent):

| Package | Responsibility |
|---|---|
| `browser` | The Chrome layer: process launch + session file (`launcher.go`), tabs (`tabs.go`), the CDP connection (`cdp.go`), and every interaction primitive (`Click`/`Fill`/`Screenshot`/`EvalJSON`/`ExtractComponents`/…). A caller can own the whole browser lifecycle through this package. |
| `observe` **(planned)** | The live-acquisition operation. Composes `browser` + `sightmap` (Corpus) + `match` + `coverage` + `render` into one annotated observation of a page. This is the "connect → extract → match → annotate" flow that ~10 commands copy-paste today. |
| `authoring` **(planned)** | The bespoke live-DOM scans used only while authoring: candidate-selector discovery (`suggest`) and link/URL-pattern discovery (`discover`). |

CLI:

| Package | Responsibility |
|---|---|
| `cmd/sightmap` | Command adapters + dispatch. Thin. |
| `skills` | Embeds the agent skills (generated mirror of the repo-root `skills/`). |

**Dependency rule:** `sightmap` (the Corpus) never imports `browser`. The
offline/online boundary runs between `sightmap`+`coverage`+`viewset` (offline)
and `observe`+`authoring` (online). `observe` is where the two halves meet.

## Curated operations (the Tier-B library API)

These are the composed entrypoints external callers reach for. Signatures are
indicative, not final.

```go
// Load a corpus once (pure, serializable data); build a Matcher (the engine
// that compiles + caches queries) to run it against live trees.
corpus, err := sightmap.Load(dir)          // *sightmap.Corpus (pure data)
m := sightmap.NewMatcher(corpus)           // *sightmap.Matcher (compiled-query cache)
m.MatchTree(root, url)                      // matches
corpus.ViewForURL(url)                      // *View

// Attach to an already-running browser (own the lifecycle yourself),
// or launch one.
conn, err := browser.Connect(addr, tabID)  // *browser.CDPConn
conn, cleanup, err := browser.Launch(ctx, browser.LaunchOptions{})

// Observe a page: the shared acquisition + annotation flow.
res, err := observe.Page(ctx, conn, corpus, observe.Options{})
// res: { Root, Matches, Props, Coverage, View }

// Pure coverage math over any tree + matches (online or offline).
cov := coverage.Score(root, matches, coverage.Options{})

// Persist an observation into a view's capture set (novelty-gated).
path, kept, err := viewset.Append(dir, view, res, viewset.Options{})
```

`snapshot` and `capture` are both thin adapters over `observe.Page`; `capture`
additionally calls `viewset.Append`. `gap` is `observe.Page` + a `coverage` T3
read (it uses no bespoke DOM JS). This is why callers never need a
`RunSnapshot([]string{...})` — they compose the operations directly.

## Command → subsystem map

| Command | Surface | Subsystems |
|---|---|---|
| `browser *` (start/stop/status/navigate/eval/click/fill/…/tabs) | browser-use | `browser` |
| `snapshot` | browser-use | `browser` → `observe` (→ `match`, `coverage`, `render`) |
| `screenshot` | browser-use | `browser` |
| `console` / `network` **(planned .12)** | browser-use | `browser` |
| `capture` | authoring | `observe` + `viewset` |
| `inspect` | authoring | `browser` + `render` |
| `suggest` / `discover` | authoring | `browser` + `authoring` |
| `gap` | authoring | `observe` + `coverage` |
| `sel-probe` | authoring | `browser` + `sel` / `compquery` + `sightmap` |
| `sel-check` | authoring (offline) | `render`/`comps` + `sel` |
| `coverage` / `multi-coverage` / `report` | authoring (offline) | `viewset` + `coverage` (+ `sightmap`) |
| `capture-novelty` / `capture-prune` | authoring (offline) | `viewset` (+ `coverage`) |
| `validate` / `lint` / `search` | authoring (offline) | `sightmap` |
| `serve-sightmap` | authoring | `sightmap` + http |
| `skills` | — | `skills` |

## Tool taxonomy: browser-use vs authoring

The command surface splits cleanly along the two agent skills:

- **Browser-use** (`sightmap-browser` skill — the drive → observe → act loop,
  no corpus authoring): `navigate`, `snapshot`, `screenshot`, `click` / `fill` /
  `hover` / `scroll` / `keypress` / `drag` / `wait-for` / `dialog`, `eval`, and
  (planned) `console` / `network`. **`snapshot` is a browser-use tool** — it is
  the default page-observation primitive and yields the component ids the
  interaction commands act on.
- **Authoring** (`sightmap-authoring` skill — everything browser-use plus corpus
  work): `capture`, `inspect`, `suggest`, `discover`, `gap`, `sel-probe` /
  `sel-check`, `coverage` / `multi-coverage` / `report`, `validate` / `lint` /
  `search`.

`snapshot` (observe) and `capture` (persist) share `observe.Page` but sit on
opposite sides of this line — matching the two-skill split exactly.

The CLI stays **flat** (no subcommand namespaces) for now; this taxonomy is the
grouping to reach for if/when flat becomes unwieldy.

## Harmonization with sibling tools

Sightmap's browser surface is designed to harmonize with two sibling tool sets:
Subtext's `subtext-live` MCP tools (the sibling product) and
`ChromeDevTools/chrome-devtools-mcp` (the upstream reference). Vocabulary:

| Concept | sightmap | subtext-live | chrome-devtools-mcp |
|---|---|---|---|
| a page/target | **tab** (`--tab`, `tabs`) | view (`live-view-*`) | page (`pageId`) |
| observe (annotated tree) | `snapshot` | `live-view-snapshot` | `take_snapshot` (a11y only, no sightmap context) |
| raw selectors (authoring) | `inspect` | `live-view-inspect` | — |
| screenshot (+ element clip) | `screenshot` (planned `component_id`/`expand_pct`) | `live-view-screenshot` (`component_id`/`expand_pct`) | `take_screenshot` (`uid`) |
| interactions | `browser <verb>` | `live-act-<verb>` | bare verbs |
| navigate / eval | `browser navigate` / `eval` | `live-view-navigate` / `live-eval-script` | `navigate_page` / `evaluate_script` |
| console / network | planned (.12) | `live-log-*` / `live-net-*` | `list`/`get_console_message`, `list`/`get_network_request` |

Intentional differences:

- **"tab", not "view".** `subtext-live` uses *view*, which collides with the
  sightmap **view** concept (a route in the corpus). Sightmap keeps *tab*; the
  `subtext-live` naming is expected to be revisited on that side (those tools are
  not yet formally launched).
- **`capture` overlap.** Sightmap `capture` = persist a snapshot into a view
  set. Subtext `capture_status` = whether Fullstory session recording is active.
  Different namespaces (our CLI vs their MCP); tolerated, called out here so docs
  don't conflate them.
- **Exceptions are not a separate tool.** Both siblings fold uncaught exceptions
  into console (error level / `Runtime.exceptionThrown`); planned `.12` does the
  same — no dedicated exceptions surface.

### Devtools scope (planned, `.12`)

Harmonized core only: `console` (list + get, level/type filter, pagination),
`network` (list + get, URL/resource-type filter, optional save-body-to-file),
and the existing `eval`.

**Non-goals for now** (chrome-devtools-mcp's profiling/QA mission, not
sightmap's): performance traces, Lighthouse, the heap-snapshot family,
screencast, and extension management. Revisitable once the core is solid.
