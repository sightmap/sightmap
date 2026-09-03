# @sightmap/sightmap

## 0.30.0

### Minor Changes

- ef6ec4b: Add `browser inject` for persistent script injection. Unlike `eval` (which runs once in the current document), `inject --persist` registers a script with the running session daemon, which re-applies it at the start of every new document in every tab via CDP `Page.addScriptToEvaluateOnNewDocument` — so it survives navigations and new tabs for the life of the session. `--file` loads the source from disk, and `--list` / `--remove ID` manage the persisted set. Useful for polyfills, overlays, and debugging/experimentation bundles that must outlive a multi-page flow.

### Patch Changes

- 72b1d3d: Docs: sync the authoring and browser agent skills to recent offline-extraction and `eval` changes. The authoring skill now states that every attribute (not just `class`/`id`) matches offline and that `text` falls back to a node's rendered `innerText` when it has no accessible name; the browser skill notes that `browser eval` awaits a returned promise.

## 0.29.0

### Minor Changes

- b8f1f03: The offline element model now captures every attribute the live DOM carries (minus injected sightmap ids and framework-internal `_`-scoping attributes like Angular's `_nghost`/`_ngcontent`), not just the curated subset that landed in the generated selector string. Offline attribute selectors (`[attr=…]`, `[attr*=…]`, …) and `extract: attr=NAME` now match the live DOM for non-standard attributes — e.g. `value` on a `role="option"` `<li>` — closing a source of live/offline divergence.
- 96babe1: Offline `text` extraction now falls back to a node's rendered text when it has no accessible name, so `extract: text` on role-less elements (custom elements, bare `<span>`s) resolves the visible value instead of silently empty. Component text is captured from `innerText` — rendered, transform- and visibility-aware, excluding `<style>`/`<script>` and non-rendered nodes — and normalized to a single clean shape (whitespace runs collapsed, ends trimmed) at the build boundary, so `snapshot`/`coverage`/`bounds` and library consumers all see the same value the runtime does. Adds a `text` field to the component-node schema.

### Patch Changes

- be65c68: `browser eval` now awaits a returned Promise and yields its settled value instead of an opaque `{}`, so async page/tool code (e.g. an awaited `executeTool()`) can be driven directly. A per-eval timeout bounds a never-settling promise. This is a no-op for synchronous expressions.

## 0.28.0

### Minor Changes

- 05e3193: Component properties now resolve offline over the component tree (SEP-0010). `Matcher.Match` populates each `ComponentMatch.Properties` from the matched tree with no live DOM, so a downstream consumer holding a serialized component tree gets property values directly. The `extract` grammar is the four tree-closed forms — `text`, `attr=NAME`, `PATH.prop`, `exists:PATH` — and `transform` is removed from component properties. `sightmap validate` now checks component `properties[]`: duplicate names within a component and unrecognized/removed extract modes are errors.
- f2b1cda: Retire the live-DOM component-property extraction pass and remove `transform` from request and message properties (SEP-0010). Component property values now come solely from the matcher's offline, tree-closed resolution — the CDP/JS extraction (`observe.ExtractProperties` and its `properties.js`) is gone, and `snapshot`/`bounds`/`query` read the values the matcher already resolved. Request and message `properties[]` keep `source`/`field`/`pattern`; fold any post-processing into `pattern` (RE2 + capture groups). `ApplyTransform` is removed.

### Patch Changes

- 1ce6f82: Port the DevTools extension's component-property extraction to the SEP-0010 tree-closed model. The embedded overlay (`resolver.js` and its inlined `content.js` copy) now resolves the four extract forms — `text`, `attr=NAME`, `PATH.prop`, `exists:PATH` — over the matched component tree, replacing the removed DOM-shaped modes (`inner_text`/`text_only`/`inner_html`/raw CSS sub-selector) and transforms. Descendant paths resolve a child component's own extracted property (`text` is the element's DOM text content, the extension's implementation-defined accessible text).
- 4f217d7: Update the `sightmap-authoring` skill for SEP-0010: component `properties[]` now document the four tree-closed extract forms (`text`, `attr=NAME`, `PATH.prop`, `exists:PATH`) and the promote-a-sub-element-to-a-child-component pattern. Removed the DOM-shaped modes (`inner_text`/`text_only`/`inner_html`/raw CSS sub-selectors) and all `transform` guidance (including the request-properties `transform` reference).

## 0.27.0

### Minor Changes

- ea91154: `sightmap browser start --detach` runs the daemon in the background (in its own session) and returns once it is serving, so scripts and agents no longer hang on the foreground daemon. Unlike `nohup start &`, the detached daemon survives the launching shell (it `setsid`s into its own session). `browser start` also now prints that it is a foreground daemon holding the shell, and `browser status` probes the sightmap HTTP server in addition to Chrome's CDP — reporting `⚠ degraded` when CDP is up but the server has been reaped, instead of a misleading `running`.

### Patch Changes

- 14979a0: `browser bounds` now accepts the full component-query grammar — property predicates and descendant chains — the same engine as `click`/`fill`/`hover`/`wait-for`. Previously it matched component names only, so `bounds 'Card[title="X"]'` or `bounds 'Row Star'` returned "matched no component". Multi-match is preserved (a query returns every matching component's box), `--substring` still does name-only fuzzy matching, and `--all` is unchanged. Also documented the existing `wait-for --component` and `--view` flags in `browser --help`.
- ba9d003: Authoring + browser skills: document the browser daemon lifecycle. `browser start` is a long-running foreground daemon that holds the shell; scripts and agents should use `browser start --detach` (which returns once serving and survives the shell) rather than `nohup start &`. Also note headless auto-detection and the `--no-sandbox` hint on sandboxed hosts, and that `browser status` can report a `⚠ degraded` daemon.
- 1da7480: `browser start` now comes up out of the box on headless and sandboxed Linux hosts. With no display (`$DISPLAY`/`$WAYLAND_DISPLAY` unset) it defaults to headless instead of dying with "Missing X server or $DISPLAY", and when Chrome's launch fails because the host restricts its sandbox (unprivileged user namespaces clamped by AppArmor or a container) the error points straight at the fix (`--chrome-flag=--no-sandbox`) rather than leaving you to decode Chrome's stderr.

## 0.26.3

### Patch Changes

- 12a5fa2: Authoring skill: document the component model. Add "Component hierarchy (`children:`)" and "Cross-view references (`$ref`)" sections covering nested components, parent-scoped selectors, the depth budget, and `$ref` attestation. The browser skill now spells out that component queries use a descendant combinator (no `>` child combinator) and that a multi-match is an ambiguity error, not a missing component.
- d2423e5: Authoring skill: fix the "Further reading" pointers. They referenced `go/README.md` (not shipped with the installed skill) and `docs/reference.md` (which does not exist). The skill is self-contained — coverage model, outer loop, tool surface, lint rules, quality checklist, and the component model are all in it — and now points to the published docs for the normative spec instead of dead repo-relative paths.
- 50478f8: Authoring skill: fill three small content gaps. Document the `memory:` shape (a list of plain strings, attachable at the file root or on a view/component), add a route-matching note (`*` matches one path segment, `**` matches zero or more, and the URL is matched decoded so `%2F` counts as a separator), and reconcile the class-selector guidance (stable hand-authored class names are fine — the volatility risk is generated/hashed classes).

## 0.26.2

### Patch Changes

- 03021a8: `sightmap multi-coverage` no longer manufactures phantom "global candidate" promotions from stale capture directories. It grouped columns purely by directory name under `snapshots/`, so a leftover or renamed dir (e.g. an old `snapshots/views/` kept alongside the current `snapshots/home/` for the same page) became a second column and made that page's own components look like they "appear in 2+ views" — advising the author to wrongly globalize view-scoped components.

  A capture dir is now treated as a real view only when it matches a view in the _current_ corpus (by `SnapBasename`). Non-current dirs are still shown in the matrix (marked `*`) for context but are excluded from the cross-view global-candidate analysis, and a warning names them so the author can delete the stale dir or re-capture under the current name. The `sightmap-authoring` skill documents the new behavior.

- 0c08758: Two authoring-clarity fixes for the offline inventory and the capture gate.

  `sightmap stats` now attributes corpus-root **global** components and requests to their own `(global) · (all views)` row instead of showing `0` against every view. A corpus whose coverage lives entirely in globals (e.g. a single-view app mapped with global components) previously rendered a per-view table that read as empty, with the confusing footer "per-view rows sum to 0". The globals are shown once, on a leading row, and the footer explains them — globals apply to every view, so folding them into one view's row would misattribute coverage. The `--json` contract is unchanged.

  `sightmap capture`'s novelty-gate message no longer reads like it is refusing a first baseline. The first capture of a view always writes (an empty set can't be redundant); reaching the gate means a baseline already exists and the new capture adds nothing, so the message now says so plainly ("<view> already has N capture(s); this one adds no new component or interactive slot — not saved"). The `sightmap-authoring` skill is updated to match and to reinforce the first-capture-always-writes guarantee.

## 0.26.1

### Patch Changes

- a6051c5: `browser install` now fetches the correct architecture on **linux-arm64**. It previously mapped every Linux host to the `linux64` (x86-64) Chrome for Testing build regardless of CPU, so arm64 machines silently got an x64 Chrome under `~/.sightmap/browsers/chrome-<ver>/chrome-linux64/`. The platform is now resolved per-arch (`linux-arm64` on arm64, `linux64` on x64). Because Google does not currently ship an arm64 build to the **Stable** channel, resolution falls back through Beta → Dev → Canary for platforms Stable does not carry, printing a one-line warning when it uses a non-Stable channel. Every mainstream platform continues to install from Stable unchanged.

## 0.26.0

### Minor Changes

- 39b63fe: Authoring efficacy on hook-poor DOMs (Salesforce Lightning, Angular, framework-generated markup):

  - **Generalized selector-candidate generation.** `gap` and `suggest` no longer dead-end when a DOM has no `data-testid`/`data-component`. Candidate generation now ranks custom-element tags, design-system classes, ids, other stable `data-*`, and `name`/`href`/`aria` hooks (data-attributes remain the top-ranked input, not an override), dropping only clearly machine-generated tokens. `gap` also emits a container hook on the hook-poor path.
  - **New `explain` command.** Node-first inspection: pick nodes by selector, `--id`, or `--grep` (role/name) — live or offline via `--snap` — and dump each node's facts, ranked selector candidates, coverage tier + owning component, and ancestor hooks. Shadow-transparent (matches the offline matcher), so authors no longer hand-read `*.snap.tree.json`.
  - **Honest coverage.** `snapshot --coverage` and `gap` now warn when a clean "0 orphaned" pass is carried entirely by global (chrome) components — i.e. the current view modeled nothing and the pass reflects a global backstop rather than a real view.
  - **Authoring skill:** a second mandatory property rule requires a per-instance discriminator on any component whose selector matches more than one instance, so repeated cards/rows/tabs stay individually addressable; plus `explain` documentation.

### Patch Changes

- 65b955b: Fix the browser overlay extension rendering **0 components** on pages whose corpus the sightmap server serves correctly. The content script proxied its `/sightmap` fetch through the background service worker, and on a cold/just-woken MV3 worker that round-trip intermittently delivered an empty corpus at page load; the empty result then stuck for the life of the page because the per-instance `version` never changes (so `pollVersion` never refetched) and an open side panel keeps the worker alive. `sightmap snapshot` was unaffected (it reads the corpus in-process over CDP). The content script now fetches the local server **directly** on http-origin pages — `host_permissions` already grants `http://localhost:*` and the server sends `Access-Control-Allow-Origin: *` — using the background proxy only as an https mixed-content fallback, and it treats an empty corpus as "not loaded yet" and keeps retrying instead of caching it.
- fffa167: Fix `snapshot`/`capture` rendering an empty component tree when the page root (or any ancestor) is reported invisible. `render.Filter` dropped an entire subtree at the first `IsVisible=false` node, but a zero-size or `visibility:hidden` ancestor — notably the document `<html>` root, which reports height 0 / ignored — can have fully visible descendants, so the whole annotated tree vanished even though coverage still counted the visible nodes. Invisible nodes are now made transparent (the node itself is omitted, visible descendants survive); a genuinely hidden `display:none` subtree still renders nothing, keeping render in lockstep with coverage.
- d07bdf6: `sightmap stats` now reports a **Messages** count. The offline inventory listed views, components, requests, properties, and memory but silently omitted the SEP-0006 `messages:` entity, so a corpus's console/exception matchers were invisible in the totals (and in `--json`). Adds `messages` to the totals table and the `--json` contract.

## 0.25.0

### Minor Changes

- 00a59e4: `browser start --attach host:port` attaches the daemon (devtools server +
  console/network collector) to an already-running Chrome's CDP endpoint instead
  of launching and owning its own. It is a deliberately degraded mode: no owned
  profile or extension guarantees, capture is complete only from attach onward
  (pre-attach network/console history can't be recovered — `Network.enable`
  replays nothing), and `browser stop` detaches rather than killing the
  caller-owned browser. The collector and devtools query surface are unchanged —
  the collector only ever needed a CDP address — so live console/exception and
  request matching work identically once attached.

  Also fixes a latent shutdown-order bug in the collector surfaced by attach mode:
  `Collector.Stop` cancelled the per-tab drain contexts _after_ `wg.Wait()`, which
  deadlocked whenever the CDP connections were still healthy at stop time (the norm
  when a consumer stops the collector without tearing down the browser). Stop now
  cancels the drains first and refuses new tab attachments during teardown.

  And fixes extension-server-port injection under attach: `findExtensionSWWS` took
  the _first_ extension service worker it found, so in a real browser with several
  extensions installed (the attach case) it injected the port hint into the wrong
  one and the sightmap overlay never learned the server port — falling back to
  probing 7891–7900 and latching onto whatever sightmap server answered first
  (often a different session). It now identifies the sightmap extension by its
  manifest name before injecting, so the correct overlay gets the hint. An owned
  launch never hit this: its isolated profile contains only the sightmap extension.

- c829831: The `browser start` devtools now surface extracted property values, not just
  matched-def names. The collector eagerly retains request/response bodies for
  XHR/Fetch traffic (request bodies inline from `requestWillBeSent`; response
  bodies fetched on `Network.loadingFinished`, off the event loop), so a buffered
  `Request` record is complete. `sightmap network list|get` annotate via
  `RequestsForRecord` — resolving each matched request's `properties[]` against the
  live headers/body — and `sightmap console list|get` surface a matched message's
  stack `properties[]`. Extracted values render as a trailing `{name=value, …}`
  token on list lines and a `Properties:` block in `get` detail, e.g.
  `GET /api/checkout/pay → 200 OK (Fetch) {outcome=declined}` — the "200 OK but the
  body says declined" case, now visible.
- 0504529: Make selectors behave consistently across shadow DOM. The captured component tree already flattens shadow roots (so offline matching, `sel-probe`'s offline count, and coverage pierce shadow), but every live-DOM operation re-found nodes with `document.querySelector`, which cannot cross shadow boundaries — so property extraction, interaction (click/fill/hover/value/scroll/wait-for), `bounds`, `sel-probe`'s live count, and `suggest`/`discover` were silently shadow-blind and disagreed with the corpus. A shared shadow-piercing resolver (`browser.DeepQueryJS`) now backs all of them, so live operations reach shadow-DOM content the same way matching does (property extraction on shadow-DOM leaves, interaction with shadow-DOM controls, and discovery of shadow-DOM links now work). `spec/v1/schema.md` formalizes the selector/tree model and its shadow-DOM semantics — a deliberate divergence from live `document.querySelector`, which does not cross shadow roots.
- 31ae338: Expand the authoring and browser skills to cover the runtime spec additions.

  - **sightmap-authoring**: new "Requests and messages" section documenting `requests:` (route/method), request `properties:` extraction (SEP-0005 `source`/`field`/`pattern`/`transform`), and `messages:` (SEP-0006 `level`/`message` + `source: stack` stack-addressing properties); a new "Runtime activity" step in the per-page loop plus a runtime line in the "Done when" checklist so the loop routes authors to these entities; and view-route-generality guidance (a catch-all route that matches every page is a smell — carve specific routes).
  - **sightmap-browser**: reframe the `console`/`network` tooling as the runtime view of the corpus — each record leads with a `[Match]`/`[--]` slot and trails extracted `{name=value}` properties — with the observe-only "reproduce the traffic" note and `--url`-substring / SPA `wait-for` caveats.
  - Fix stale command references: `sel-probe -- 'selector'` (the bare `sel-probe 'selector'` form fails) and `gap --include-hidden` (the documented `--visible` flag does not exist).

### Patch Changes

- 782c4d6: The reference network/console collector now populates the faithful-record fields
  added for SEP-0005/0006 runtime matching. Observed `Request` records carry
  request/response headers (from the CDP `requestWillBeSent`/`responseReceived`
  events — cheap, no extra round-trip) and an observed `DurationMs`; observed
  exception `Message` records carry the `Stack` (from `exceptionThrown`
  `stackTrace.callFrames`, throwing frame first, with 0-based line/column
  preserved). Bodies remain fetched on demand. This is what lets
  `RequestsForRecord` extract body/header properties and `MessagesForRecord`
  resolve stack properties against traffic the reference tooling captured.
- c22792c: Make `network get` reliable and header-aware. The devtools body endpoint now serves the response/request body eagerly retained on the record (captured at `loadingFinished`) instead of always re-fetching it from Chrome via `getResponseBody`, which failed once the browser had evicted the body ("response body no longer available"). And `network get` now renders the captured request/response headers (they were already on the record from the collector but never surfaced). This closes the HTTP response header/body residual split out of the network-collector gaps.

## 0.24.0

### Minor Changes

- f20e1de: `match.Matcher` gains a chain-matching entry point for consumers that classify a
  stream of individual observed elements rather than a full component tree. Each
  observed element carries only its own root→leaf ancestor chain, so there was no
  first-class API for it — a consumer had to reimplement the spec's component
  identity/tag resolution by hand (and route-blind). `MatchChain(chain, pageURL)`
  runs the shared NFA matcher over a single-branch spine built from the chain and
  returns depth-annotated `ChainMatch` values; `NamesForChain` applies the
  nearest-enclosing identity rule and `TagsForChain` the tag-union rule (deduped,
  sorted), so the spec's resolution lives once in the shared library. Matching is
  route-aware, so view-scoped components apply on the chain exactly as in a
  full-tree match. Purely additive packaging over already-exported pieces — no new
  matching semantics.

## 0.23.0

### Minor Changes

- cc92928: `messages:` can now match and extract on exception **stack traces** (a SEP-0006
  follow-on). An observed `Message` gains an optional `Stack []Frame` (throwing
  frame first; `Frame{Function, File, Line, Column}`), empty for a plain console
  record. A `MessageDef` gains a stack-addressing `properties[]` mirroring
  SEP-0005's `source`/`field`/`pattern`/`transform`: the one source is `stack`,
  `field` names a frame and attribute (`top.file`, `top.function`, `1.line`, where
  `top` aliases frame `0`), an optional RE2 `pattern` refines the resolved value,
  and `transform` post-processes it. `Corpus.MessagesForRecord` now folds the
  extracted values into each `MessageMatch.Properties`, bringing it to parity with
  `ComponentMatch`/`RequestMatch`; unresolved values are omitted silently, so a
  plain console record (no stack) simply extracts nothing. The reference CLI
  validates the new declarations (`message-property-invalid-name` /
  `-source-invalid` / `-no-field` / `-pattern-invalid`).
- af5f844: Observed `Request` records can now carry the payload a `RequestDef`'s
  `properties[]` addresses, and a new matcher resolves them — the runtime half of
  SEP-0005. `Request` gains optional (omitempty) `DurationMs`, `ReqHeaders` /
  `RspHeaders` (`[]Header`), and `ReqBody` / `RspBody` (`*Body`) fields, so a
  producer holding a full capture can populate them while a lazy producer leaves
  them nil and the wire stays lean. `Corpus.RequestsForRecord(rec) []RequestMatch`
  does route+method identity matching (via `RequestsForURL`) and then resolves each
  matched def's `properties[]` against the record — `source` → `field` (JSON
  dot-path with numeric array indexing for a `*.body`; case-insensitive header
  lookup for a `*.headers`) → optional RE2 `pattern` (capture group 1 else the whole
  match, or a scan of the raw source when `field` is absent) → optional
  `transform`. Unresolved values are omitted silently per the spec, so an
  incomplete record degrades to fewer properties rather than erroring.

## 0.22.0

### Minor Changes

- 3f2d8dd: Add `sightmap export [dir]`: load a `.sightmap/` corpus and emit the canonical
  Corpus wire — the exact `json.Marshal(sightmap.Corpus)` shape a library consumer
  reads (`selectors[]` arrays, components nested under each view, plus `requests`
  and `messages`) — to stdout, a file (`-o FILE`), or an HTTP endpoint (`--url`,
  POSTed as `application/json` with no auth headers). The `.sightmap/` directory is
  auto-detected by walking up from `[dir]` or the cwd, and TLS verification is
  skipped for local hosts (`localhost`, `127.0.0.1`, `.test`) or under an explicit
  `--insecure`. A companion `sightmap push URL [FILE]` POSTs a corpus JSON (from a
  file or stdin) through the same transport.

  This replaces the hand-rolled Python collector (`collect_and_upload_sightmap.py`)
  that shipped a second, lossy serializer — it flattened views into compound
  selectors and dropped routes, requests, and messages. Routing the upload through
  the Go loader makes it the single source of truth and shares the exact
  `sightmap.Corpus` type with the server-side reader, so the two ends cannot drift.

### Patch Changes

- d15dd73: `sightmap serve` and the `browser start` daemon now serve the canonical Corpus
  wire — the same shape library consumers see (`selectors[]` arrays, components
  nested under each view) — inside a thin `{site, version, corpus}` envelope,
  replacing the bespoke pre-compiled shape. The bundled browser extension consumes
  it directly; cache-busting via `/sightmap/version` is unchanged.

## 0.21.0

### Minor Changes

- 424a2c1: Restructure the Go library for downstream consumers. All corpus vocabulary,
  observed runtime records, and match-result types now live in a single
  self-contained `sightmap` package, with the matching engines consolidated behind
  one `match.Matcher`. Types follow a consistent naming model — `…Def` for a spec,
  bare structs for observed records, `…Match` for results — and extracted values
  share a typed `PropertyValue`. This is a breaking change to the Go import surface
  for library consumers (there were none before this release).
- 424a2c1: Add runtime matching for console/exception messages. `Corpus.MessagesForRecord`
  classifies an observed console record against `messages:` definitions
  (case-insensitive level equality + an RE2 `message` regex, precompiled at load),
  returning every match so an ambiguous record is surfaced rather than silently
  resolved. The `sightmap console` and `sightmap network` devtools listings now
  annotate each captured record with the corpus definitions that classify it —
  messages via `MessagesForRecord`, requests via route+method identity — leading
  each line with the match.

## 0.20.0

### Minor Changes

- 8828f5c: Add a top-level `messages:` entity ([SEP-0006](https://github.com/sightmap/sightmap/blob/main/spec/seps/0006-message-entity.md)): named console-output and exception patterns, matched by `level` and a `message` regex. This gives console activity what `requests:` gives network activity, a named entity the rest of the corpus can reference by name.

  **An uncaught exception arrives as `level: exception`, not `level: error`.** The reference capture emits `log`, `debug`, `info`, `warn`, `error`, and `exception`, so `level: ERROR` matches `console.error` and does not match an exception. SEP-0006 still needs no `kind:` discriminator, because the origin is carried as a level value, but a corpus that wants exceptions has to name them.

  New validation:

  - `message-regex-invalid` (error) compiles `message` at validation time, matching how component selectors are already checked. The corpus no longer stores a pattern nobody has proven is a pattern. The dialect is pinned to **RE2** (Go `regexp` / the `re2` npm package for JS): a linear-time syntax with no backreferences or lookaround, so authoring-time validation and runtime matching agree across SDKs.
  - `merge-collision-message` (warning) reports a duplicated name. This one is load-bearing for SEP-0007: `ref:` resolution counts distinct entity kinds, so two messages sharing a name collapse to one kind and the ambiguity check never fires.
  - `message-conflict` (warning) reports two entries that can match the same record, where that overlap is statically decidable: same `level`, identical or absent `message`.
  - `message-level-unknown` (lint) catches a level outside the emitted vocabulary. The realistic trap is `WARNING`, CDP's own spelling, which the capture normalizes to `warn`.
  - `field-type-invalid` (error) now covers message fields, so `message: 404` and `level: 500` are rejected in Go as ajv already rejected them.

  This resolves SEP-0006's open question on ambiguous-match diagnostics, which the SEP delegated to its implementation.

  Matching is not implemented: the SDK parses and validates these declarations but does not evaluate them against console records, even though the capture layer already collects both console output and exceptions.

- 01f0a37: Add `properties:` to request definitions ([SEP-0005](https://github.com/sightmap/sightmap/blob/main/spec/seps/0005-request-properties.md)): named values a consumer extracts from a live request/response pair, so a `200 OK` whose body says a payment was declined can be reasoned about.

  - `source` names the root (a closed enum: `req.body`/`rsp.body`/`req.headers`/`rsp.headers`).
  - `field` selects a value within `source`: an object-key dot-path for a body source, a header name for a headers source (required there).
  - `pattern` is an **RE2** regex (Go `regexp` / the `re2` npm package for JS; no backreferences or lookaround) that refines what `field` resolved, or scans the raw source text when `field` is absent. `field` and `pattern` compose (`anyOf`) instead of being mutually exclusive.
  - `transform` shares the component-property vocabulary (unchanged; cleanup tracked separately).

  New validation, closing a gap where the Go SDK accepted corpora the JSON Schema rejected:

  - `request-property-invalid-name`, `request-property-no-extractor`, `request-property-source-invalid`, `request-property-headers-require-field`, and `request-property-pattern-invalid` (errors) enforce in Go what only ajv enforced before.
  - `request-property-shadows-reserved` (warning) fires when a property is named `status`, `method`, or `duration`, which shadows the request's HTTP identity and makes it unreachable from a signal filter.
  - `field-type-invalid` (error) rejects an unquoted non-string scalar in a schema-string field. yaml.v3 decodes any scalar into a Go string by taking the raw lexeme, so `source: 200` used to load as `"200"` while ajv rejected it.

  Extraction itself is not implemented: the SDK parses and validates these declarations but resolves no `source`/`field`/`pattern` and applies no transform. `spec/v1/schema.md` marks the evaluation requirements as such, and the `018-request-properties` conformance fixture is now executed by the Go test suite.

## 0.19.0

### Minor Changes

- c1da8b4: Add `sightmap atlas`: `find` searches the community atlas at sightmap.org/atlas, `list` browses it, and `add` installs one corpus into `.sightmap/`.

  `find` matches slug, name, domains, categories, and description, ranked so an exact domain match comes first, because an agent about to automate a site starts from a URL and has no way to guess a slug. Each hit prints its own `sightmap atlas add` command, and a query that matches nothing exits 0.

  `add` fetches one `.tar.gz` and never reads the index, so an index outage cannot stop an install. It refuses a non-empty target before touching the network; there is no `--force`. `--index` and `--source` point the verbs at a mirror or a private corpus store, under the same HTTPS-only transport policy and the same archive caps.

  New public API: `sightmap.Totals`, the five corpus-wide counts split out of `sightmap.Stats`, which now embeds it. Field access and the `sightmap stats --json` output are unchanged — the JSON is byte-identical — but a composite literal must now write `Stats{Totals: Totals{Views: 1}}`. The split exists because a published catalog entry carries these five numbers as its `stats` object and puts the per-view rows in a sibling field, so `atlas.Entry.Stats` is a `sightmap.Totals`: one definition of what the counts mean, and no `PerView` field that no valid catalog could ever fill. As a side effect `sightmap atlas find --json` now prints a count that is zero rather than omitting it, and reports `properties` and `memory`, which it previously discarded.

- c1b62b7: Add corpus statistics, in the library and on the CLI.

  New public API: `sightmap.Stats`, `sightmap.ViewStats`, and `(*Corpus).Stats()`, so any consumer of a loaded corpus — the atlas index generator, Subtext — gets the counts without shelling out to the CLI. `Components` counts distinct component names corpus-wide (a global reused by three views is one component), while `Properties` and `Memory` are summed over distinct component _definitions_, so a `$ref`-expanded copy counts once but two views that each define a different component under the same local name both count. `Stats.IsEmpty` reports a corpus with nothing in it — memory alone is not nothing.

  New `sightmap stats` verb over that API: the totals plus a per-view component/request table, and `--json` for a stable machine-readable form (`views` / `components` / `requests` / `properties` / `memory` / `per_view`) suitable for CI consumers. `stats` refuses a corpus that `sightmap validate` rejects, since the loader drops the definitions it cannot resolve and the counts would silently under-report; in `--json` mode the failure is itself JSON (an `error` key plus `diagnostics`), so a consumer always has something to parse.

## 0.18.0

### Minor Changes

- 61d097c: **Breaking:** rename the rule-side corpus types to the `Def` convention — `match.SightmapComponent` → `match.ComponentDef` and `match.SightmapMatch` → `match.ComponentMatch`. A `ComponentDef` describes what to match; a `ComponentMatch` is the result of matching one. No behaviour change; callers update to the new names.
- 5b5c019: Add `sightmap init`: scaffold a schema-correct `.sightmap/` corpus (a commented `components.yaml` and `views/example.yaml` carrying `version: 1` and the top-level `views:` wrapper) so the first files an author sees are already valid instead of written from memory. Existing files are never overwritten, and the scaffolded corpus passes `sightmap validate` as-is.
- f8c01f1: **Breaking:** split the matching engine out of `Corpus`. `Corpus` is now pure, serializable data; build a `Matcher` with `NewMatcher(corpus)` to match a live component tree. `Corpus.MatchTree` and `Corpus.Components` move to `Matcher.MatchTree` / `Matcher.Components` (the per-URL compiled-query cache now lives on the `Matcher`). Read-only corpus queries (`ComponentsForURL`, `ViewForURL`, `RequestsForURL`, `GlobalComponentNames`) stay on `Corpus`.
- 4dd830a: Model `requests:` in the corpus and make it serializable to a stable wire form:

  - Global and view-scoped API request definitions (`RequestDef` / `Payload` / `Field`) are now parsed into the corpus — previously `requests:` was known to the schema but silently dropped.
  - New `Corpus.RequestsForURL(url, method)` returns every request whose route glob (and optional method) matches the observed request — "all matches apply", per the spec. Reuses the existing route matcher (`:param`, trailing-slash, `**`).
  - `Corpus`, `View`, and `RequestDef` now carry JSON tags, so a corpus serializes to a lean wire form (`memory` / `globals` / `views` / `requests`); authoring-only `View` fields (`url` / `access` / `snapshots` / `sourceFile` / `stability`) are excluded. The flattened list is emitted in a stable pre-order (lexical file order, then declaration-order depth-first), locked by a test, so the wire form is reproducible.

### Patch Changes

- d67da96: First-run papercuts fixed:

  - Subcommand `--help`/`-h` now exits 0 and no longer prints `flag: help requested` — a help request is a success, not an error (#117).
  - `browser status` on a session file that exists but doesn't parse (e.g. hand-written or corrupted, so no valid `port`) now reports it as an unrecognized format with the expected shape, instead of a misleading "server and CDP were assigned the same port (0)" collision hint. The collision hint is also gated on a real server port (#118).
  - `validate` no longer warns about unknown fields in tooling files it owns (`survey.yaml`), as it already skips `config.yaml` (#119).

- 5a0e2c6: Make `browser start` work — and fail legibly — on Linux and in root containers (the standard agent/CI environment):

  - `FindChrome` now checks the sightmap-managed Chrome for Testing install (`~/.sightmap/browsers/`) on Linux and Windows, not just macOS, so the documented `browser install` → `browser start` flow works. The "no Chrome found" errors now point at `browser install`.
  - `browser start` adds `--no-sandbox` automatically when running as root (Chrome refuses to start otherwise), and accepts `--chrome-flag` (repeatable) and `--chrome-binary` to override the launch.
  - On a startup timeout, `browser start` now reports the resolved binary, the full argument list, and the tail of Chrome's stderr — instead of a bare "timed out waiting for CDP" that hid the real cause.

- 0405fcc: Three first-run papercuts:

  - `validate` now errors on an unsupported `version:` value (e.g. `version: 2`) — the spec requires `version: 1`. This is the companion to the missing-`version:` warning.
  - `browser install --help` prints usage and exits 0 instead of ignoring its arguments and starting the 184 MB Chrome-for-Testing download (the subcommand now parses its flags).
  - `search` names the near-miss when a pattern matches a **view** or **request** name (search covers component fields only), instead of a bare "no matches".

- 319d472: Route matching: `**` is now a proper globstar. As a whole path segment it matches zero or more segments, so `/admin/**` matches `/admin` itself (as the spec always stated) as well as `/admin/users` and deeper, and `/a/**/b` matches `/a/b`. A `**` glued into a segment (e.g. `/foo**`) is treated as a regular single-segment `*` that does not cross a `/`. This fixes the previous behaviour where `/admin/**` failed to match `/admin` and `/messages**` failed to match `/messages`.
- 332f813: Remove the authoring skill's instruction to run `sightmap browser register --addr localhost:PORT` — that subcommand does not exist, so agents following it dead-ended. Attaching to an externally-launched browser remains a possible future feature (tracked upstream); until it lands the skill no longer documents it.
- e4a7269: `report` and `capture` errors now teach the `views:` file structure instead of naming a field in isolation:

  - `report` distinguishes "no views defined at all" from "views exist but none has a `url:`", and both print a minimal `views:` example (with `route:` and `url:` and their roles) — previously it always said "no views with URLs found / Add a url: field", which contradicted `capture`'s route-only advice.
  - `capture`'s "no view matches" error shows the same `views:` example and names `route:` explicitly.

  The `sightmap-authoring` skill's Phase 1a now shows a complete view-file example (the top-level `views:` list with `version:`/`route:`/`url:`), instead of only telling authors to "create a view file with `route:`" — which invited putting view fields at the file root (silently making it a globals file).

- 599e3d5: Teach the corpus schema at the point authors get it wrong, instead of failing silently or generically:

  - A **view field at the file root** (e.g. a top-level `route:`/`name:`) now warns that view fields belong under a top-level `views:` list, with a short example, instead of a bare "unknown field".
  - A **view-shaped file** that sets `url:` and `components:` but no `views:` now warns that it defines no views and its components are treated as global (previously it validated clean and silently became a globals file).
  - A **missing `version:`** now warns (the spec requires `version: 1` in every corpus file).
  - `validate` now warns when the **whole corpus has global components but no views** — it can never match a view, so capture, report, and per-view coverage are unavailable.

  All are advisory warnings (exit 0); correct corpora stay warning-free.

## 0.17.0

### Minor Changes

- e5ccfb8: Go library: components can now carry `source:` (relative path to the implementing file — already schema-recognized, previously dropped by the loader) and `tags:` (authored classification labels, e.g. `defect`). Neither is inherited by children, matching `memory`/`properties`/`stability`'s existing convention. `Tags` also flows through `ApplySightmap`'s match result alongside `Memory`. New `Corpus.AllComponents()` returns every component in the corpus (globals plus every view's), deduped by first-seen name — the flat, whole-corpus list a consumer building an upload payload or a lint/coverage report wants, replacing an equivalent hand-rolled loop in `cmd_lint.go`.

## 0.16.1

### Patch Changes

- a24d165: The release workflow now pushes a `go/vX.Y.Z` tag alongside the existing `vX.Y.Z` tag. This module lives in the repo's `go/` subdirectory rather than at the root, and Go's module versioning requires a nested module's tags to be prefixed with that subdirectory — so `go get github.com/sightmap/sightmap/go@vX.Y.Z` has never actually resolved to a tagged release, only to `@latest`'s branch-tip pseudo-version. The bare tag is unchanged and still drives everything else (goreleaser, npm publishing, the release-already-tagged check).

## 0.16.0

### Minor Changes

- 4dfc0a9: Go library: `Corpus.Memory` now carries file-level `memory` entries (the loader previously dropped them). Lower the module's `go` directive from 1.25.2 to 1.23, its actual dependency floor, so consumers aren't forced onto a newer toolchain than the code requires.

## 0.15.10

### Patch Changes

- 882499d: Verify the changesets release automation end-to-end: no functional change.

## 0.15.9

### Patch Changes

- ac54cc2: Offline selector matching now matches the live DOM for `id`, `class`, and SVG. Three gaps are closed so a selector `sel-probe` verifies live behaves the same way `snapshot`/`coverage`/`capture` see it offline:

  - **Attribute selectors on `id`** (`[id^="issue_"]`, `[id$=…]`, …) now match. `id` lives in a dedicated node field, so the matcher resolves `id`/`class` attribute selectors to those fields — not only to captured `attrs`.
  - **`placeholder`** is captured, so `input[placeholder="…"]` matches offline.
  - **SVG classes** are captured. On SVG elements `className` is an `SVGAnimatedString`, which broke the old extraction (it threw and dropped the whole selector); `probe.js` now uses `classList`, so `svg.lucide`, `[class*="lucide"]`, and `:has(svg.lucide-x)` match offline.

  The authoring skill and docs are corrected accordingly: attribute selectors on `class` and `id` are no longer described as "don't work offline" — they work, for HTML and SVG alike.

- b19e770: Better handling of asynchronous SPA navigation, without baking implicit waits into `click`.

  - **`navigate` now reports client-side redirects.** It previously only saw server-side (HTTP) redirects, which are reflected by the load event; a client-side redirect that fires during hydration (an auth guard bouncing `/login → /`, or `/ → /workspace`) went unreported, so the caller was told it was somewhere it wasn't. `navigate` now waits briefly after load for a follow-up navigation and prints `(redirected to FINAL)` for those too.
  - **`wait-for` gains `--view` and `--component`.** These are the explicit, corpus-aware step boundaries to use after an action that should navigate (the act-then-wait split Playwright and Selenium use): `--view <Name>` waits until the current URL resolves to a named sightmap view, and `--component '<Query>'` waits until a component query — including property filters like `WorkItemRow[key="FALCON-7"]` — matches a node. Both auto-retry until they hold or time out loudly. `--url`, `--selector`, and `--load` remain the raw equivalents.

  `click` deliberately does **not** wait for or guess about resulting navigation — it acts, reports, and keeps its loud covered/off-screen refusals. Adds `browser.AwaitNavigation` (waits for `Page.frameNavigated` / `Page.navigatedWithinDocument`, settling chained redirects) behind `navigate`.

## 0.15.8

### Patch Changes

- 20c8d15: Coverage and the annotated tree now agree on what counts as visible. `probe.js` set each node's `isVisible` from that element's _own_ computed style, which misses ancestor-driven hiding: a control inside a closed `opacity:0` overlay (a dismissed dropdown or context menu) has computed `opacity:1` of its own, so it reported visible even though it is painted nowhere. Coverage counted these while the renderer dropped the hidden container's subtree, so on real apps a large share of a page's interactive nodes (all the closed menus) silently inflated the count and tanked the score.

  `probe.js` now computes `isVisible` with the browser's own `Element.checkVisibility`, which accounts for ancestors hidden by `display:none`, `visibility:hidden`, `opacity:0`, or `content-visibility` — keeping the layout/rendering judgment in the real browser instead of re-deriving CSS semantics. `snapshot`/`coverage` "(visible only)" now excludes descendants of hidden containers; `--include-hidden` still counts every interactive node. The renderer also drops invisible subtrees regardless of interactivity so it and coverage read one signal.

## 0.15.7

### Patch Changes

- e1e4310: `snapshot` now surfaces runtime match conflicts in a `[Conflicts]` section — the ambiguities that only exist against a live page, complementing the static corpus-conflict warnings `validate` emits. Two triggers:

  - **A single DOM node matched by more than one distinct component name.** Matching is first-match-wins, so only one applied and the others were silently dropped — which is also why a correct-looking component can report `0 matches`. The section names the node, the competing components, and which one won.
  - **Two or more views matching the current URL at equal specificity**, where declaration order alone decided the winner.

  Adds `match.FindConflicts` and `Corpus.TiedViews`; both are computed during `observe.Page`. (The view-tie half is provisional pending the route-specificity decision in the functional-decomposition proposal.)

- d5ccb5a: `validate` now surfaces two classes of invalid corpus the loader previously dropped silently (so they shipped with a green check):

  - **Missing required fields** — a component with no `selector` (or no `name`), and a view with no `name`, are now errors (`missing-selector` / `missing-name` / `missing-route`) instead of being quietly discarded during load.
  - **Unresolved `$ref`** — a `$ref` naming a component that no file defines is now a `ref-unresolved` error (matching the spec's MUST), instead of the reference being silently skipped.

  Both exit non-zero. The spec's diagnostic-code table documents the new error codes alongside the corpus-conflict warnings.

- 8a16957: `validate` now warns on unknown fields. The typed loader silently ignored any YAML key it didn't recognize, so a typo like `memroy:` — or a half-baked field — vanished without a trace. `validate` now walks the raw YAML and emits an `unknown-field` **warning** (not an error) for any key the spec doesn't define at its position, at any nesting depth. It warns rather than rejects, so authors can stash experimental fields (e.g. `macros:`) during development; recognized fields — including the reserved tooling fields `access` and `snapshots` — are never flagged, and `.sightmap/config.yaml` is excluded. This completes strict `validate` alongside the earlier required-field and `$ref` checks.
- 5be278e: Fix `report` and make the view `url:` field first-class. `url:` is now read **per view** (with a file-level `url:` as a default for views that omit their own), instead of only at the file level. Previously per-view `url:` was silently dropped, so `report` — which needs a representative URL per view — errored with `no views with URLs found` even when every view declared one. `url:` is also now part of the published schema (SEP-accepted alongside `properties`), so a corpus that declares it validates instead of failing `additionalProperties: false`.

## 0.15.6

### Patch Changes

- 774637d: `validate` now distinguishes **errors** (which fail validation, exit 1) from **warnings** (advisory, exit 0), and warns on silent corpus conflicts that a fallback rule was resolving without telling you:

  - **`merge-collision-view`** — two or more views share a `name`.
  - **`route-conflict`** — two or more views share the same (normalized) `route`; only the first-declared applies to that URL (this is the "same-route hijack" that can silently drop a view's components).
  - **`merge-collision-component`** — two or more root-level global components share a `name` with different selectors.

  Findings now carry a stable `code` and `severity`. Scoped component name reuse (the same child under multiple parents) is correctly _not_ flagged, since that is intentional. The spec's diagnostic-code table documents the new corpus codes.

- 0ed063e: Make `snapshot` loud about corpus-match state instead of silently omitting it. When a corpus is loaded but no view's route matches the URL (e.g. an auth redirect to a login page), the output now shows a `[No view matched] <url>` notice instead of a headerless tree that looks identical to a normal snapshot — so an agent knows the page is off the map. And the `[Coverage]` summary is now printed whenever a corpus is applied, including for a matched view that has no components yet (every interactive node reads as an orphan); previously that case printed only the view header, leaving the documented `snapshot --coverage` bootstrap with nothing to iterate on.

## 0.15.5

### Patch Changes

- 04c6216: Make `browser click` and `fill` fail loudly instead of silently no-op'ing. `click` now scrolls the target to the center of the viewport and verifies its center is the top-most element there before dispatching — it errors when the target can't be positioned in the viewport (previously an off-screen target below the fold was clicked at a coordinate that hit nothing and still exited 0) or is covered by another element (an open overlay/modal). The success confirmation reports the real post-scroll coordinates. `fill` now reads the value back after typing and errors when a non-empty value was typed but the field is still empty — the signature of a React-controlled input where plain typing doesn't stick — telling you to retry with `--clear`.
- 307839a: `sel-probe` now cross-checks the offline matcher. It queries the live DOM as before, but also runs the same selector through the offline matcher that `snapshot`/`coverage`/`capture` use, prints that count, and emits a `⚠ offline/live divergence` warning (with a hint) when the two disagree. This closes the false-confidence trap where a selector "verified" against the live DOM is silently dead in the corpus — e.g. attribute selectors on `id` (`[id^="…"]`) match live but never offline, because `id` is a dedicated node field rather than a matchable attribute.

## 0.15.4

### Patch Changes

- ce30028: Make browser-use feedback loud: the interaction commands (`click`, `fill`, `hover`, `keypress`, `scroll`, `drag`, `wait-for`, `dialog`, `tabs resize`) now echo a short confirmation on success instead of exiting silently — e.g. `clicked CartNavButton @ (674,30)` — so an agent driving the browser can see what happened. Several raw CDP/Go errors are now rewritten into actionable messages: a `wait-for` timeout reads `timed out after 800ms waiting for selector "..."` (not `context deadline exceeded`), resolving a dialog when none is open says so plainly, and fetching an evicted network response body explains that bodies are only retained briefly. `snapshot` now prints a note when no `.sightmap` corpus is found at the target directory instead of silently rendering an un-annotated tree.

## 0.15.3

### Patch Changes

- 7c3b990: Stop green-lighting broken corpora and blank pages:

  - `snapshot` now exits non-zero when the corpus is present but fails to parse (a missing corpus stays non-fatal — the tree still renders), and when the observed page has 0 interactive nodes (blank or still loading). It still renders whatever it observed first. `inspect` and `suggest` now warn on a bad corpus instead of silently ignoring it.
  - Coverage no longer marks a page with 0 interactive nodes as a pass. The coverage line renders a distinct `∅` mark (instead of `✓`), the `coverage` command counts empty captures as failures, and `capture` refuses to persist a blank/loading page as a view's baseline (override with `--force`).

## 0.15.2

### Patch Changes

- 4c175e8: Make corpus loading robust against two malformed inputs that previously failed badly:

  - A circular `$ref` chain (`A → B → A`, or a component that references itself) sent every corpus-loading command into infinite recursion and hung the process. Loading now detects the cycle, stops expanding, and `validate` reports it as a `ref-circular` error instead of hanging.
  - `splitSelectors` only balanced parentheses, so a comma inside an attribute selector or quoted string (`[data-x="a,b"]`) was wrongly treated as a selector-list separator and split into two dead alternatives. Splitting is now aware of `[]` brackets and quoted strings (with backslash escapes), matching how CSS is actually written.

- 27cb85a: Fix view route matching so it behaves as the spec documents. Trailing slashes are now normalized off both the route pattern and the URL path before matching, so a slash-free route like `/*/projects` matches the `/acme/projects/` paths that Django, Rails, and many React-Router apps emit — previously such URLs matched no view at all, silently dropping the `[View:]` header, view memory, and view-scoped components. Express-style `:param` segments in view routes now match a single path segment (and score between a literal and `*` for specificity). And `Corpus.ViewForURL` now returns the _most specific_ matching view — using declaration order only to break equal-specificity ties — instead of the first match in corpus order, closing a divergence between the exported library and the spec.

## 0.15.1

### Patch Changes

- b04eab6: Isolate concurrent browser sessions so agents working in different projects no longer cross-talk. The session file is now keyed to `--sightmap-dir` (it lives at `<sightmap-dir>/.session`) instead of a single shared `$TMPDIR` file, so a second `browser start` only ever reuses the Chrome for _its own_ corpus rather than piggybacking on another agent's browser. Free-port probing now checks the IPv4 loopback (where Chrome's CDP and the sightmap server bind), and the sightmap server binds `127.0.0.1`, so two daemons started on the default ports slide to non-overlapping ports instead of one daemon's server colliding with another's CDP port. Every session-aware `browser` command (including `console`/`network`, `status`/`stop`, `tabs`, and the low-level interactions) now accepts `--sightmap-dir` to select which session it talks to.
- 7dba2e8: Fix the Sightmap overlay getting stuck in a reload loop that flooded the captured console. The extension's version poll compared the raw `/sightmap/version` JSON text against the parsed version string, so it never matched and re-fetched every few seconds. The poll now parses the response and compares the `version` field. Also drop the post-install `chrome.runtime.reload()` step: a fresh `browser start` always relaunches Chrome with `--load-extension`, which loads the new unpacked extension directly, and the hot-reload could leave the overlay's content script uninjected until the next full restart.

## 0.15.0

### Minor Changes

- 96a1cdb: CLI teardown: reorganize the tooling around reusable library subsystems and add browser devtools.

  - Separate `snapshot` (observe: annotated tree + coverage) from `capture` (persist into a view's set); extract the `observe`, `coverage`, `viewset`, and `authoring` packages and merge the internal `Session` into `Corpus`.
  - Make the `browser start` daemon the single session owner: retire `--launch` and the `browser launch` subcommand (live commands attach to a started session).
  - Add `console` and `network` devtools commands, backed by a session-lifetime collector in the daemon that buffers console messages (with uncaught exceptions folded in) and network requests, with lazy response bodies.
  - Add screenshot clipping to `browser screenshot` (`--component` / `--selector` / `--expand-pct`).
