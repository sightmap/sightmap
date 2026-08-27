# webmcp/ — the sightmap → WebMCP codegen adapter

Turn any publicly accessible website into a set of **WebMCP tools** an agent
can call, by way of its sightmap corpus. This directory holds the codegen
adapter: a compiler from `.sightmap/` (views, components, properties,
requests, memory) plus a small hand-authored tool manifest to a
self-contained JS bundle that registers tools on
[`document.modelContext`](https://github.com/webmachinelearning/webmcp) —
the W3C Web Machine Learning CG's WebMCP proposal, in origin trial in Chrome
149 / Edge 150 and supported by ChatGPT Desktop and Brave Leo.

## The pipeline

The end-to-end goal: **site URL in, distributable WebMCP implementation
out**, with an agent in the loop exactly where judgment is needed and
mechanical, verifiable steps everywhere else.

```
 any public site
       │
       │  1. MAP — sightmap atlas add <slug>, or an agent authors the corpus
       │     against the live site (sightmap-authoring skill: coverage loop,
       │     verified selectors, per-instance properties, requests, memory)
       ▼
 .sightmap/ corpus
       │
       │  2. SCAFFOLD — sightmap-webmcp init drafts one read tool per view
       │     and one api stub per corpus request
       ▼
 webmcp.tools.yaml
       │
       │  3. REFINE — an agent walks the site's real user goals with the
       │     sightmap browser CLI (trajectories, network traces), picks each
       │     tool's kind (api / fetch flow / live flow), and folds corpus
       │     memory into tool descriptions (sightmap-webmcp skill)
       ▼
       │  4. GENERATE — sightmap-webmcp generate compiles manifest + corpus
       │     into a bundle (compile-time resolution of every component,
       │     property, view, and request reference; errors, not guesses)
       ▼
 site.webmcp.js / .module.js / .user.js
       │
       │  5. VERIFY — inject the snippet into a live sightmap browser
       │     session and execute every tool via the built-in shim
       │
       │  6. DISTRIBUTE — userscript for any WebMCP-enabled browser agent,
       │     ES module for the site's own owners, snippet for harnesses;
       │     regenerate on corpus change (generate --check gates drift)
       ▼
 tools callable by any WebMCP browser agent
```

Steps 1 and 3 are agent work, guided by the `sightmap-authoring` and
`sightmap-webmcp` skills. Steps 2, 4, 5, 6 are this package.

## Why route through a sightmap?

WebMCP assumes the _site owner_ writes the tools. Third-party sites mostly
won't, for a while — but an agent with the sightmap CLI can map any site
today. The corpus is precisely the knowledge a good tool needs:

| Corpus entity                                                      | Becomes                                                                      |
| ------------------------------------------------------------------ | ---------------------------------------------------------------------------- |
| `views:` (route globs, representative URLs)                        | navigation targets, `require_view:` guards, URL templates                    |
| `components:` + `children:` (verified selectors, shadow-flattened) | act/read targets, resolved by semantic name                                  |
| `properties:` (`extract` + `transform`)                            | structured reads and query predicates (`Row[label="X"]`)                     |
| `requests:` (+ request `properties:`)                              | api-backed tools with response extraction ("200 but the body says declined") |
| `memory:`                                                          | the hazard lore folded into tool descriptions                                |
| coverage discipline (0 orphaned, sel-probed)                       | tools that keep working because the map is maintained                        |

The manifest references corpus entities **by name**; the compiler inlines
selectors and extractors at generate time and fails loudly on anything that
doesn't resolve. The corpus stays the single source of truth — improve the
map and every regenerated tool improves with it.

## Tool kinds

A WebMCP tool's `execute` runs inside the page, so the fundamental
constraint is: **a tool call dies with the document**. The three kinds work
around that honestly:

- **`api`** — replay an observed endpoint with `fetch` (first-party cookies
  included, so signed-in state comes free). Method and result extractors
  inherit from the corpus `requests:` entry. Preferred whenever one request
  does the job. Cross-origin APIs that answer with
  `Access-Control-Allow-Origin: *` — the norm for key- or bearer-authenticated
  REST APIs, Supabase and Firebase among them — cannot be called credentialed
  at all, because the browser rejects a wildcard origin on a credentialed
  response; set `credentials: omit` for those.
- **`flow` + `mode: fetch`** — fetch a server-rendered page with cookies,
  parse it off-screen (DOMParser), and read components from the detached
  document. Cross-"page" reads with no navigation. Useless on client-rendered
  content — verify SSR first.
- **`flow`** (live) — fill/click/wait/read on the live DOM by component
  identity, gated by `require_view:` (off-view calls return a structured
  error with a `navigate_to` hint). `navigate` is legal only as the final
  step; the generator rejects mid-flow navigation at compile time.

## Layout

```
webmcp/
  bin/sightmap-webmcp.js   CLI: init | validate | generate [--check]
  src/
    corpus.js              .sightmap/ loader ($ref expansion, breadcrumb index,
                           view scoping, selector chains)
    manifest.js            webmcp.tools.yaml structural validation
    query.js               component-query DSL parser (CLI-compatible subset)
    globs.js               spec route-glob → regex
    compile.js             manifest + corpus → self-contained tool IR
    emit.js                IR → snippet / module / userscript (deterministic,
                           corpus-hash provenance banner)
    scaffold.js            init: corpus → draft manifest
    runtime/runtime.js     the embedded browser interpreter (shadow-piercing
                           query, extraction, actions, fetch/api executors,
                           modelContext registration + __sightmapWebMCP shim)
  test/                    jest suite (runs in the repo root's jest config)
  examples/ikea/           worked example against the vendored IKEA atlas
                           corpus: manifest + all three generated bundles
  examples/github/         second worked example (GitHub's legacy + React
                           frontends, fetch flows + an api tool); its
                           verify-live.js executes every tool against real
                           github.com — run it manually, CI stays hermetic
```

The runtime's deep-query and extraction functions are adapted from the
reference implementation's own browser-side helpers (`go/browser/deepquery.js`,
`go/observe/properties.js`), so generated tools resolve nodes and values the
way the CLI that authored the corpus did.

Generated bundles always install `window.__sightmapWebMCP`
(`listTools()` / `callTool()` / `callToolAndStore()` for eval bridges that
can't await) alongside `document.modelContext` registration — that's the
verification surface, and the fallback for browsers without the origin
trial.

## Try it

The generator ships inside the `sightmap` CLI (Go, published to npm as
`@sightmap/sightmap`) as `sightmap webmcp <init|validate|generate>` — a port
of this package under `go/webmcp/` that emits **byte-identical** bundles
(golden tests in `go/webmcp/golden_test.go` regenerate the committed examples
from Go and byte-compare). This directory remains the reference
implementation and the test harness for the shared browser runtime.

From the repo root:

```bash
# validate + regenerate the worked example (either entry point)
sightmap webmcp validate --tools webmcp/examples/ikea/webmcp.tools.yaml
node webmcp/bin/sightmap-webmcp.js generate --tools webmcp/examples/ikea/webmcp.tools.yaml --format all

# scaffold a fresh manifest from any corpus
sightmap webmcp init --site myshop --base-url https://myshop.example \
  --sightmap-dir path/to/.sightmap --out webmcp.tools.yaml

# tests
npx jest webmcp                 # generator + runtime (jsdom) + example drift
(cd go && go test ./webmcp/)    # Go port incl. byte-parity golden tests
```

The full authoring loop — including live verification via
`sightmap browser eval` — is in [`skills/sightmap-webmcp/`](../skills/sightmap-webmcp/SKILL.md).

## Manifest reference

See the [skill](../skills/sightmap-webmcp/SKILL.md) for the working
reference and the [IKEA example](examples/ikea/webmcp.tools.yaml) for a
fully-worked manifest. In brief:

```yaml
version: 1
site: slug # required
base_url: https://site.example # required; resolves relative URLs
sightmap: .sightmap # corpus path, relative to this file
match: [https://site.example/*] # userscript @match (default: origin/*)
tool_version: "0.1.0" # userscript @version
tools:
  - name: tool_name # ^[a-z][a-z0-9_-]{0,63}$
    description: ... # required — the agent decides by it
    read_only: true # default: GET api / interaction-free flow
    view: ViewName # scope component-name resolution to a view
    require_view: ViewName # live flows: runtime guard + view scope
    params:
      - {
          name: q,
          type: string|number|integer|boolean,
          required: true,
          description: ...,
          enum: [...],
          default: ...,
        }
    api: # exactly one of api | flow
      request: CorpusRequestName # inherits method + result properties
      url: "https://.../{q}" # {param} URL-encoded; {param|raw} opts out
      query: { k: "{q}" } # appended query params
      headers: { accept: application/json }
      body: { k: "{q}" } # object → JSON (typed leaves); string → raw
      credentials: include # include (default) | same-origin | omit
      rows: # project a JSON array body into per-row fields
        field: data # path to the array (default: the whole body)
        max: "{limit}"
        fields:
          title: { field: title } # dot path within the row
          creator: { field: user.display_name }
          url: { template: "/item/{row.id}" } # {param} then {row.path}
      result: # request-property vocabulary
        - {
            name: outcome,
            source: rsp.body,
            field: status,
            pattern: "...",
            transform: "...",
          }
    mode: fetch # flow only: fetch | live (default)
    flow:
      - navigate: "/path?q={q}" # fetch: first step; live: final step only
      - wait_for: { component: Name, timeout_ms: 10000 } # or selector/url_includes
      - fill: { target: "Query or css:SEL", value: "{q}" }
      - click: 'Row[label="{q}"] Buy'
      - press: Enter # optional target:
      - sleep: 500
      - scroll: { to: Name } # or { delta_y: N }
      - read:
          key: { component: Name, property: prop } # corpus extractor
          key2: { component: Name, extract: attr=href } # inline extractor
          key3: {
              selector: ".x",
              extract: text,
              transform: first_dollar,
              max_chars: 2000,
            } # per-field cap (default 300)
          rows:
            for_each: Name # or a component query
            max: "{limit}"
            fields:
              a: { property: prop } # iterated element's prop
              b: { component: Child, property: p } # descendant, chain-relative
              c: { selector: a, extract: attr=href }
```

An api tool with `rows:` answers `{status, rows}` instead of echoing the raw
body — the array projected into exactly the fields an agent needs, with
`template:` composing values the API itself does not return (a page URL built
from a row id, say). It is the api-side counterpart of a flow read's
`for_each`, and it is usually what turns a raw API response into a usable
tool result.

Reads follow sightmap's silent-omission convention: a value that doesn't
resolve is absent, never an error. Every read value caps at 300 chars
unless the field sets `max_chars`; api bodies echo up to `max_body_chars`
(default 20000) when no `result:` is declared.

## Trust model

Generated tools run with the page's authority, so be clear about who trusts
whom:

- **The manifest and corpus are trusted inputs.** Whoever generates a bundle
  is asserting the tools it contains: URL templates decide where credentialed
  (`credentials: "include"`) fetches go, and flows decide what gets clicked.
  Review both the way you'd review code — especially a corpus installed from
  the atlas (`atlas add` proves it loads, not that it's benign).
- **Params are agent-supplied.** They are URL-encoded into URLs by default;
  `{param|raw}` opts out, and the compiler warns when a template parameterizes
  the URL's _origin_, because that hands the destination host to the caller.
- **The shim grants no new privilege.** Any script on the page can already do
  everything a tool does (same-origin DOM + fetch); `window.__sightmapWebMCP`
  adds a convenient surface, not a capability. The mediated surface is
  `document.modelContext`, where the browser controls which agents see which
  tools. Mark mutating tools `read_only: false` so hosts can gate them.
- **A userscript user trusts the publisher.** Installing `SITE.webmcp.user.js`
  runs it on every matching page with their session. Publish reproducibly:
  the banner's corpus hash lets anyone regenerate and diff.
- **Embedded strings cannot break out.** Authored text is JSON-embedded with
  `</` escaped (safe to inline in a `<script>`), and banner/userscript header
  lines are collapsed to one line.

Runtime limits worth knowing: every read caps at 300 chars — `for_each`
row fields included — unless the field sets `max_chars`; un-`result:`ed
api bodies cap at `max_body_chars`; regex
patterns evaluate as JS RegExp — stay in the RE2∩JS subset the authoring
skill prescribes; fetched documents are decoded as UTF-8; `press` synthesizes
events (keydown/keyup + keyCode for common keys) that some handlers ignore —
prefer URL-shaped actions.

## Status & non-goals

- The generator targets the current WebMCP shape (`document.modelContext`,
  `registerTool`, `execute` returning any JSON-serializable value,
  `annotations.readOnlyHint`) and also probes the older
  `navigator.modelContext` location. As the proposal evolves,
  `src/runtime/runtime.js` (`__smwBoot`/`__smwDescriptor`) is the single
  place to track it.
- Cross-document journeys inside one tool call are out of scope by design
  (so is anything the WebMCP spec itself still leaves open — streaming,
  elicitation, cross-document responses).
- The user-facing entry point is `sightmap webmcp ...` in the published CLI;
  this npm package stays repo-internal (`private: true`) as the reference
  implementation. The two are kept byte-identical by the golden tests, so a
  change to either generator's output must land in both (and regenerate
  `examples/`).
