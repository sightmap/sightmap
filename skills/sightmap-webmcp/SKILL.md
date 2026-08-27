---
name: sightmap-webmcp
description: Use when turning a mapped site into WebMCP tools — walking user-goal trajectories with the sightmap browser CLI, authoring a webmcp.tools.yaml manifest against the .sightmap/ corpus, generating a publishable document.modelContext tool bundle, and verifying it in the live session. Activate when the goal is to build, generate, or publish WebMCP tools for a website.
activation:
  - building or publishing WebMCP tools for a site
  - webmcp.tools.yaml present in project
---

# Sightmap → WebMCP: generate in-page tools for any site

WebMCP lets a page expose **tools** to AI agents via
`document.modelContext.registerTool({name, description, inputSchema, execute})`
(W3C Web Machine Learning CG proposal; origin trials live in Chrome 149 and
Edge 150, supported by ChatGPT Desktop and Brave Leo). Sites that register
tools let agents act through one reliable call instead of a fragile
click-path. Almost no third-party site does this yet — but if a site has a
sightmap corpus, you already hold everything a tool needs: verified
selectors, per-instance properties, view routes, API endpoints, and the
hazard lore in `memory:`.

This skill is the loop that turns that corpus into a **generated, publishable
WebMCP bundle**: map → walk trajectories → author a tool manifest → generate
→ verify live → publish. The codegen adapter lives in the sightmap repo under
`webmcp/` (`sightmap-webmcp` CLI). Each generated bundle registers its tools
with `document.modelContext` where available **and** always installs
`window.__sightmapWebMCP` — a shim with the same tools, used for verification
below and by non-WebMCP browsers.

```
.sightmap/ corpus  ──┐
                     ├─ sightmap-webmcp generate ──► site.webmcp.js        (snippet — inject & verify)
webmcp.tools.yaml  ──┘                              site.webmcp.module.js  (ES module — site owners)
   (you author this)                                site.webmcp.user.js    (userscript — third-party publish)
```

## Phase 0: a corpus, and the codegen CLI

- **Corpus**: `sightmap atlas find <domain>` → `atlas add`, or author one with
  the `sightmap-authoring` skill. Every component/property/request a tool
  references must exist in the corpus — the manifest names corpus entities,
  it never contains raw selectors except as a deliberate `css:` escape hatch.
- **Codegen**: ships in the `sightmap` CLI you already have:
  `sightmap webmcp <init|validate|generate>`. (The reference implementation
  lives at `webmcp/` in the sightmap repo as a standalone Node CLI —
  `node webmcp/bin/sightmap-webmcp.js` — and the two emit byte-identical
  bundles; use whichever is at hand.)

## Phase 1: pick tools by walking trajectories

A tool is a **user goal**, not a page. "Search products", "what does this
item cost", "add to cart" — not "click the third button". For each candidate
goal, perform the trajectory once with the `sightmap-browser` skill and
record what the tool will need:

1. **Drive it**: `browser navigate` → `snapshot` → `click`/`fill` by
   component query → `wait-for`. Every component query you use here
   transfers verbatim into the manifest (`Row[label="X"] Buy`).
2. **Watch the network while you do it**: `sightmap network list --type XHR`,
   then `network get <index>` on anything that carried the real work. If one
   request does the whole job (a search API, a stock endpoint), the tool
   should probably call **it** rather than drive the DOM. Corpus `requests:`
   entries with `properties:` are pre-wired result extractors.
3. **Note what varied**: the query text, the category id, the article number
   — those are the tool's `params`. Corpus `memory:` often tells you how
   params map to URLs ("the article number ends the product slug").
4. **Note the outcome signal**: a component property that changed, or a
   response-body field (`{first_action_state=SUCCESS}` style) — that is the
   tool's read-back / `result:`.

Then choose each tool's kind — this decision is the heart of the manifest:

| Kind                   | When                                                  | Why                                                                                                                                                                                                                                                                                                                                                 |
| ---------------------- | ----------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `api`                  | one observed endpoint does the job                    | Most reliable; runs with the page's cookies (first-party auth for free). Cross-subdomain calls need CORS — check with `network get` that the browser itself made the call from this origin. An API answering `Access-Control-Allow-Origin: *` (key/bearer auth — Supabase, Firebase) cannot be called credentialed at all; give it `credentials: omit`, and note it then runs unauthenticated. |
| `flow` + `mode: fetch` | read-only, and the target page is **server-rendered** | Fetches the URL with cookies, parses it off-screen (DOMParser), reads components — works from any page, never navigates. Verify SSR first: corpus memory often says ("everything here is already in the document"), else `browser eval` a `fetch(url).then(r=>r.text())` and check the markup is there. Client-rendered content is invisible to it. |
| `flow` (live)          | mutating or same-page interactions                    | Fills/clicks on the live DOM by component identity. **A WebMCP tool call dies with the document**, so a live flow must stay on one page — `navigate` is only legal as the _final_ step, and cross-page journeys must be split into per-page tools gated by `require_view:`.                                                                         |

## Phase 2: author `webmcp.tools.yaml`

Scaffold, then edit: `sightmap webmcp init --site SLUG --base-url URL`
drafts one fetch-read tool per corpus view and one api stub per corpus
request — keep what maps to real goals, delete the rest.

```yaml
version: 1
site: ikea
base_url: https://www.ikea.com
sightmap: .sightmap # path to the corpus, relative to this file
tools:
  # description is REQUIRED and load-bearing: fold the relevant memory:
  # hazards into it — inside a folded scalar a "#" is literal text, so keep
  # comments out here.
  - name: search_products # snake_case; the agent-facing tool name
    description: >
      Keyword search... The summary count is the authoritative total —
      the grid renders only the first page.
    read_only: true
    mode: fetch
    view: SearchResults # scopes component-name resolution to a view
    params:
      - { name: query, type: string, required: true, description: Keywords. }
      - {
          name: max_results,
          type: integer,
          default: 20,
          description: Card cap.,
        }
    flow:
      - navigate: "/us/en/search/?q={query}"
      - read:
          summary: { component: SearchSummary, property: summary }
          products:
            for_each: ProductCard # every match, scoped fields below
            max: "{max_results}"
            fields:
              text: { property: card_text } # iterated element's prop
              price: { component: CardPrice, property: price_text } # child
              url: { selector: a, extract: attr=href } # raw escape hatch

  - name: get_buyback_offers
    description: Buy-back offers for one article number.
    params:
      - {
          name: article,
          type: string,
          required: true,
          description: Digits only.,
        }
    api:
      request: CircularOffers # corpus request — method + result props inherited
      url: "https://web-api.ikea.com/circular/circular-asis/offers/articles/{article}"
```

Semantics that do the heavy lifting:

- **Everything resolves through the corpus.** `component:` values are
  component names or component queries (`Row[label="{x}"] Buy`, predicates
  `=`/`^=`/`*=` + ` i`, `#N` index, whitespace descendant — same DSL as the
  CLI). `property:` pulls the corpus-declared `extract`/`transform`. The
  compiler errors on anything that doesn't resolve (ambiguous names list
  their breadcrumb candidates; missing properties list what the component
  does declare) — fix the manifest or improve the corpus, never paste
  selectors around the model.
- **`view:` disambiguates per-view names.** IKEA's `ProductCard` subtree
  differs on search vs category — its `CardPrice` child resolves to a
  different selector per view; a tool sets `view: SearchResults` and gets
  that view's definition (globals stay visible). `require_view:` implies it for live
  flows and adds the runtime guard: off-view calls return a structured
  `{error, expected_view, navigate_to}` instead of flailing (`navigate_to`
  resolves the view's route against the origin the page is on, so it points at
  the deployment in hand rather than the corpus's capture URL).
- **Page values reach the signed-in session.** `credentials: include` only
  authenticates a cookie session. Where the app sends a header instead (most
  SPAs), a header or query value may be read from the page:
  `{from: local_storage|session_storage|cookie|dom, key|selector, json, prefix}`
  — e.g. `authorization: {from: local_storage, key: sb-auth, json: access_token,
  prefix: "Bearer "}`. The tool then acts as the signed-in user, which is the
  thing an in-page tool can do that a server-side one cannot. A tool that reads
  page state must pin its URL origin (the compiler errors otherwise), and the
  value is redacted from `req.headers` so no `result:` can leak it.
- **`rows:` shapes an api tool's output.** A raw API body is rarely the right
  tool result: `rows: {field, max, fields}` projects a JSON array into named
  per-row fields, where `field:` is a dot path into the row and `template:`
  composes a value the API never returned — `url: {template: "/item/{row.id}"}`
  turns an id into a link. With `rows:` the tool answers `{status, rows}`
  rather than echoing the body.
- **`api` tools inherit the corpus request's `method` and `properties:`** as
  their result extractors (`source`/`field`/`pattern`/`transform` — the
  "200 OK but the body says declined" machinery). No `result:` and no
  request properties → the tool echoes `{status, url, body}` (body capped).
- **Templates**: `{param}` in URLs (URL-encoded; `{param|raw}` opts out),
  query/header/body values, predicate values, and `max:`. A JSON body leaf
  that is exactly `"{param}"` keeps the param's number/boolean type.
- **Descriptions are the product.** Distill the corpus `memory:` hazards the
  agent needs ("a bad id silently redirects to the whole catalog — check
  category_name") into each tool's description. That lore is why a generated
  tool beats a naive one.
- Missing reads are **silently omitted** (sightmap's own convention) — say in
  the description what absence means when it means something.

## Phase 3: generate and verify in the live session

```bash
sightmap webmcp validate --tools webmcp.tools.yaml
sightmap webmcp generate --tools webmcp.tools.yaml --format all
```

Fix every validate error before generating — they are compile-time proof
that each tool's selectors, properties, views, and params all resolve.

Then prove the tools work where they will run, in the sightmap browser
session (the shim makes this possible in any Chrome, no origin trial
needed):

```bash
sightmap browser start --detach
sightmap browser navigate 'https://www.ikea.com/us/en/'
# inject the generated snippet into the page:
sightmap browser eval "$(cat ikea.webmcp.js)"
sightmap browser eval '__sightmapWebMCP.listTools().map(t => t.name)'
# call a tool; the eval bridge can't await, so park the result and poll:
sightmap browser eval '__sightmapWebMCP.callToolAndStore("search_products", {query: "desk chair"})'
sightmap browser eval '__sightmapWebMCP.last'          # repeat until done: true
```

Judge the result like an agent would: is the answer complete, is a failure
actionable, does an empty result distinguish "none" from "wrong page"?
Iterate manifest → generate → re-inject until every tool passes on the live
site. Re-verify mutating tools on a page where mutation is acceptable.

## Phase 4: publish

- **Userscript** (`site.webmcp.user.js`): the third-party path — installable
  via Tampermonkey/Violentmonkey and publishable (Greasy Fork etc.); it
  registers the tools on every matching page. `@match` comes from the
  manifest's `match:` (default: the base_url origin).
- **ES module** (`site.webmcp.module.js`): hand to the site's owners —
  `<script type="module" src="...">` makes the site itself WebMCP-enabled,
  the outcome the proposal actually wants. Suggest it upstream when the site
  has a public repo or feedback channel.
- **Snippet** (`site.webmcp.js`): for injection by agent harnesses (as in
  Phase 3) or extensions.
- Commit the manifest next to the corpus and check the generated bundles'
  freshness in CI with `generate --check` (exit 2 on drift). Regenerate
  whenever the corpus changes — the bundle's banner carries the corpus
  content hash, so drift is detectable, and the map staying honest is what
  keeps the tools honest.

## Hard rules

- **Never hand-edit a generated bundle** — edit the manifest or the corpus
  and regenerate.
- **Never ship a tool you didn't execute** against the live site in Phase 3.
- **Component queries in manifests must come from the corpus** (or be
  explicit `css:` escapes). If a query can't disambiguate an instance, the
  corpus is missing a property — fix it there (see `sightmap-authoring`,
  property rules), and every future tool benefits.
- **Live flows never navigate mid-flow** (the generator rejects it): a
  WebMCP tool call cannot outlive its document. Split cross-page journeys
  into per-page tools with `require_view:`; the agent (or the final
  `navigate` step) moves between pages.
- **Mark mutating tools** `read_only: false` and say in the description what
  they mutate — hosts surface `readOnlyHint` to users.
- **The manifest is trusted input** — its URL templates decide where the
  page's credentialed fetches go. Review an installed corpus before
  generating from it, keep api origins fixed (the compiler warns when a
  template parameterizes the host), and publish userscripts reproducibly:
  the banner's corpus hash lets anyone regenerate and diff.
