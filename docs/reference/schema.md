---
# AUTO-GENERATED — DO NOT EDIT BY HAND.
# Generated from spec/v1/schema.md by docs/scripts/sync-spec.mjs.
# Regenerate: node docs/scripts/sync-spec.mjs
title: "Sightmap Schema Reference"
sidebarTitle: "Schema"
description: "Exhaustive field-level reference for the Sightmap v1 YAML format, generated from the canonical spec."
---


> **Status**: spec stream `1`, project semver **0.1.0** (pre-1.0). In-place tightening allowed until the project hits 1.0.0. See [the versioning policy](/reference/versioning).

A sightmap is a directory of YAML files at the root of a project, under `.sightmap/`. It describes the app's **views**, **components**, and **requests**, with optional **memory** entries that carry notes agents can use at runtime.

This document is the human-readable reference. The machine-readable contract is [`sightmap.schema.json`](https://github.com/sightmap/sightmap/blob/main/spec/v1/sightmap.schema.json).

## File discovery

- Every `*.yaml` and `*.yml` file under `.sightmap/` is discovered recursively.
- All files are loaded and merged at load time. The directory layout is a convenience for authors; it has no semantic meaning.
- Every file must begin with `version: 1`.
- Merging is shallow-append per top-level collection (`views`, `components`, `requests`). Two files may define the same view; the runtime behavior in that case is implementation-defined and SDKs SHOULD emit a warning.

## File root

```yaml
version: 1
memory:      # optional, string[] — file-level notes (see "Memory")
views:       # optional, View[]
components:  # optional, Component[] — global, matched on every view
requests:    # optional, Request[] — global, matched on every view
messages:    # optional, Message[] — console/exception patterns
```

| Field | Type | Required | Description |
|---|---|---|---|
| `version` | integer | yes | Must be `1`. |
| `memory` | string[] | no | File-level memory entries. Surfaced as context to the agent for any view in this file. |
| `views` | [View](#view)[] | no | Views defined in this file. |
| `components` | (Component \| [ComponentRef](#component-references))[] | no | **Global** components — matched against every view. Entries may be either inline definitions or `$ref` reference objects. |
| `requests` | [Request](#request)[] | no | **Global** requests — matched against every view. |
| `messages` | [Message](#message)[] | no | Console-output and exception patterns. Corpus-root only; there is no view-scoped form. |

## View

A named screen in the app, identified by a URL route.

```yaml
- name: FlightSearch
  route: /search
  description: Main search page with date and origin/destination pickers
  source: src/pages/FlightSearch.tsx
  memory:
    - The search form lives inside a modal on mobile; selectors differ
  components: [...]
  requests: [...]
```

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | yes | Shown in the snapshot header. Should be unique across the sightmap. |
| `route` | string | yes | Glob pattern matched against the URL pathname. See [Route matching](#route-matching). |
| `url` | string | no | Representative URL for this view — a concrete address that resolves to it. Tooling uses it to navigate to the view (e.g. coverage reporting and bulk capture/probe). A file-root `url` supplies a default for every view in the file that omits its own. |
| `stability` | string | no | Authoring-confidence marker: `stub` or `deferred`. See [Stability](#stability). |
| `description` | string | no | Free-text. Not surfaced at runtime but useful for PR review and future maintenance. |
| `source` | string | no | Relative path to the source file. |
| `dependencies` | string[] | no | Supplementary files (minimatch globs, project-root-anchored, `!` negates) whose changes should trigger re-curation of this view. See [Dependencies](#dependencies). |
| `memory` | string[] | no | View-level memory entries. |
| `tags` | string[] | no | Open-vocabulary classification labels for this view. See [Tags](#tags). |
| `components` | (Component \| [ComponentRef](#component-references))[] | no | View-scoped components. Additive with globals (but a view-scoped `$ref` subsumes the matching global for that view — see [Component references](#component-references)). |
| `requests` | [Request](#request)[] | no | View-scoped requests. Additive with globals. |

## Component

A named DOM subtree, identified by one or more CSS selectors.

```yaml
- name: DepartureDatePicker
  selector: '[data-picker="departure"]'
  source: src/components/DatePicker.tsx
  description: Departure date picker, calendar + typed input
  memory:
    - Accepts typed YYYY-MM-DD — skips the calendar
    - Past dates render but are aria-disabled
  children:
    - name: date-input
      selector: input
    - name: day
      selector: '[role="gridcell"]'
```

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | yes | Replaces the generic a11y role in enriched snapshots. |
| `selector` | string \| string[] | yes | CSS selector, or a list of alternatives. First match wins. |
| `source` | string | no | Path to the source file. Rendered inline as `[src: …]` in enriched snapshots. |
| `dependencies` | string[] | no | Supplementary files (minimatch globs, project-root-anchored, `!` negates) whose changes should trigger re-curation of this component. See [Dependencies](#dependencies). |
| `description` | string | no | Free-text. Not surfaced at runtime. |
| `memory` | string[] | no | Component-level memory entries. |
| `stability` | string | no | Authoring-confidence marker: `uncertain` or `unstable`. See [Stability](#stability). |
| `properties` | [Property](#component-properties)[] | no | Named DOM-value extractions surfaced in enriched snapshots (e.g. `[Card price="$10"]`). Extracted from the live DOM at snapshot time; unavailable to offline tools. See [Component properties](#component-properties). |
| `tags` | string[] | no | Open-vocabulary classification labels for this component. See [Tags](#tags). |
| `children` | (Component \| [ComponentRef](#component-references))[] | no | Nested components. Child selectors are scoped to the parent's subtree. Entries may be either inline definitions or `$ref` reference objects. |

### Component references

Any entry in a `components:` array — at file root, within a view, or under `children:` — may be a **reference object** instead of an inline definition:

```yaml
- $ref: ComponentName
```

A reference is expanded inline (deep copy) to the named component's full definition before matching. The name is resolved against a **registry** built from the root-level `components:` arrays of all loaded files. Nested children of an inlined definition are themselves re-expanded if they contain further `$ref` entries.

| Field | Type | Required | Description |
|---|---|---|---|
| `$ref` | string | yes | Name of a component defined at file root. The entry MUST contain no other keys. |

**Lookup scope.** Only components defined at the **root** of some file's `components:` array are addressable. Components nested under `children:`, or defined inside a view's `components:`, are not in the registry. First-seen wins on duplicate names (sorted by source-file path); SDKs SHOULD emit a `merge-collision-component` warning.

**Conformance.** SDKs MUST expand `$ref` entries before matching, MUST emit `ref-unresolved` (error) for an unknown name, MUST emit `ref-circular` (error) for a self-referential chain, and MUST NOT produce two matches for the same view when a view-scoped `$ref` and a file-root global share a name (the view-scoped expansion subsumes the global for that view).

See [SEP-0002](https://github.com/sightmap/sightmap/blob/main/spec/seps/0002-component-ref.md) for the full proposal and rationale.

### Selector semantics

- A single string is a standard CSS selector.
- A list of strings is tried in order; the first selector that matches wins.
- Selectors in `children` are evaluated **within** their parent's matched subtree, not globally. This scoping is how Sightmap avoids naming collisions between, say, two different card components that both contain a `button.primary`.
- Selectors are not required to be unique at their level. If a selector matches multiple elements, all matches are named.


### Component properties

A component may declare `properties: Property[]` — named values extracted from its matched DOM element and surfaced alongside the component name in enriched snapshots, e.g. `[DateFilterButton label="This Weekend"]`. Extraction operates on the exact element the selector matched (not an ancestor, not the AX node). Properties are read from the **live DOM at snapshot time**; SDKs operating on saved component-tree data MUST omit the values rather than approximate them (the declarations are not an error offline). When a component's selector matches multiple elements, extraction runs independently on each. See [SEP-0003](https://github.com/sightmap/sightmap/blob/main/spec/seps/0003-component-properties.md).

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | yes | Key used in the annotation (`name="value"`). Must match `^[a-z][a-z0-9_]*$`. The name `value` is reserved: it may be declared to override the AX built-in, and SDKs MUST prefer a declared `value` over the AX tree's own value. |
| `extract` | string | yes | Extraction directive (see below). |
| `transform` | string | no | Optional post-processing applied to the extracted string (see below). |

**Extract modes.** The `extract` value is interpreted in this order (named modes match by exact, case-sensitive equality before the `attr=`/`exists:` prefixes):

| Value | Source |
|---|---|
| `text` | `element.textContent.trim()`, internal whitespace collapsed to single spaces |
| `inner_text` | `element.innerText.trim()` — layout-aware; use when `text` runs adjacent inline elements together |
| `text_only` | `textContent` after removing `img`, `svg`, and `[alt]` descendants — use when icon alt-text bleeds into the label |
| `inner_html` | `element.innerHTML` |
| `attr=NAME` | `element.getAttribute(NAME)` |
| `exists:SEL` | `"true"` if `element.querySelector(SEL)` matches; the property is **omitted** when it does not (boolean state flag) |
| *any other string* | Treated as a CSS sub-selector: `element.querySelector(VALUE)?.textContent.trim()`; omitted when it matches nothing |

**Transforms.** When `transform` is set it is applied to the trimmed extracted string; if the value is empty or absent, the transform is skipped and the property omitted. Exactly one transform may be given (not composable in v1).

| Value | Effect |
|---|---|
| `first_word` | First whitespace-delimited token |
| `last_word` | Last whitespace-delimited token |
| `first_number` | First substring matching `\d[\d,.]*` |
| `first_dollar` | First substring matching `\$[\d,.]+` |
| `number` | Strip all non-digit, non-decimal characters |
| `slug` | Lowercase; spaces and underscores → hyphens; strip non-`[a-z0-9-]` |

**Value omission is silent** — a property that extracts to an empty string (or whose `exists:`/sub-selector finds nothing) is simply dropped from the annotation; consumers MUST NOT treat omission as an error.

## Dependencies

The `dependencies` field on a view or component declares supplementary files whose changes SHOULD trigger re-curation of the entry. It is purely curation-time metadata — runtime consumers (browser-driving agents, session-replay enrichers) read DOM/runtime state, not source files, and MUST NOT introduce page-load runtime cost on the basis of this field.

### What belongs

- Hooks the view or component consumes (e.g. `useChecklist`, `useAuth`)
- Services / stores / shared utilities
- CSS / style files the entry loads
- Helper modules that don't warrant their own entry

### What does NOT belong

- Tests (`*.test.ts`, `*.spec.tsx`)
- Type-only imports
- Framework code (React, Vue, etc.)
- Files that have their own component or request entry — use the existing entry-level binding, don't restate

### Glob semantics

Strings in `dependencies` are interpreted as minimatch globs, project-root-anchored (the directory containing `.sightmap/`). A `!` prefix negates. When multiple positive globs match the same file, the first positive glob in declaration order wins for provenance reporting.

### Normative rules

A conforming SDK MUST surface a diagnostic when:

1. An entry's resolved `dependencies[]` set contains its own `source`. Diagnostic code: `dependencies.self-redundant`.
2. An entry's resolved `dependencies[]` set contains a path that is the `source` of any other entry in the same `.sightmap/`. Diagnostic code: `dependencies.overlaps-entry`.
3. A glob in `dependencies[]` resolves to zero files. Diagnostic code: `unknown-source` (existing vocabulary, narrowed to apply to `dependencies[]` globs).

## Request

A named API endpoint.

```yaml
- name: SearchFlights
  route: /api/flights/search
  method: POST
  description: Run a flight search and return results
  source: src/api/flights.ts
  request:
    fields:
      - name: origin
        type: string
      - name: destination
        type: string
      - name: departureDate
        type: string
        description: ISO-8601 date
  response:
    fields:
      - name: results
        type: array
  headers: [x-request-id]
  memory:
    - 429s on more than 10 requests/min per user
```

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | yes | Shown in network list and detail output. |
| `route` | string | yes | Glob pattern. Express-style `:param` segments are normalized to `*`. See [Route matching](#route-matching). |
| `method` | string | no | HTTP method filter (`GET`, `POST`, …). Match-any if omitted. |
| `description` | string | no | What the endpoint does. |
| `source` | string | no | Relative path to the source file. |
| `request` | [Payload](#payload) | no | Expected payload shape. |
| `response` | [Payload](#payload) | no | Expected response shape. |
| `headers` | string[] | no | Notable header names to highlight in the network detail view. |
| `memory` | string[] | no | Request-level memory entries. |
| `tags` | string[] | no | Open-vocabulary classification labels for this request. See [Tags](#tags). |
| `properties` | [RequestProperty](#request-properties)[] | no | Values to extract from live traffic. |

### Request properties

`properties:` declares named values to pull out of a live request/response pair, so a consumer can reason about what an endpoint's traffic actually said. An HTTP status of `200` does not distinguish an approved payment from a declined one when the outcome lives in the response body.

```yaml
- name: CheckoutPayment
  route: /api/checkout/pay
  method: POST
  properties:
    # A JSON body value: `field` is an object-key path within `source`.
    - name: outcome
      source: rsp.body
      field: status

- name: CheckoutRetryPayment
  route: /api/checkout/pay/retry
  method: POST
  properties:
    # A header value refined by a regex: `field` names the header, `pattern`
    # extracts a substring from what it resolves to.
    - name: rate_limit_remaining
      source: rsp.headers
      field: X-RateLimit-Remaining
      pattern: '(\d+)'

- name: LegacyCheckoutCallback
  route: /api/checkout/callback
  method: POST
  properties:
    # No `field`: the response is form-encoded, so there's no JSON body to
    # traverse — `pattern` scans the raw source text directly.
    - name: legacy_outcome
      source: rsp.body
      pattern: '(?:declined|approved|deferred)'
```

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | yes | Key a consumer refers to this value by. Must match `^[a-z][a-z0-9_]*$`. |
| `source` | string | yes | Which root to read from: `req.body`, `rsp.body`, `req.headers`, or `rsp.headers`. |
| `field` | string | see below | The value to select within `source`. For a `.body` source, an object-key dot-path (a numeric segment indexes an array when the value there is one — `items.0.name`). For a `.headers` source, a header name matched case-insensitively; **required** whenever `source` is a headers source. |
| `pattern` | string | see below | An [RE2](#regular-expressions) regex applied to whatever `field` resolved, or to the raw source text when `field` is absent. Capture group 1 is the extracted value when the pattern has one, otherwise the entire match. |
| `transform` | string | no | Same vocabulary as [Component properties](#component-properties)' transform. |

At least one of `field`/`pattern` is required — the two compose: `field` selects a value, `pattern` optionally extracts a substring from it. `source` is always required, and when it names a headers source `field` is required too (a bare regex across a raw header block is the addressing foot-gun this shape removes). `pattern` is an [RE2 regular expression](#regular-expressions); the reference CLI rejects an invalid one (`request-property-pattern-invalid`).

**Value omission is silent** — a property that doesn't resolve (a missing key, an out-of-range index, no pattern match) is simply absent; consumers MUST NOT treat omission as an error. Omission is the normal case, not an edge case: whether a body or header is even available to read depends on the capture layer's own payload and privacy settings.

Extraction requires **live traffic**. A tool operating on static corpus definitions alone MUST treat `properties:` as declared-but-unavailable, not an error.

`status`, `method`, and `duration` are **reserved identity names**, addressing the request's own already-structured HTTP identity. They sit outside `source` entirely — a consumer may reference them wherever a property name is expected with no `properties:` declaration at all. Declaring a property under one of those names is legal and shadows the identity: the name then resolves to the extracted value, and the HTTP identity becomes unreachable. The reference CLI warns (`request-property-shadows-reserved`). Prefer a distinct name such as `outcome` unless shadowing is what you want.

`properties:` and `request:`/`response:` (Payload) answer different questions: `Payload.fields[]` documents expected shape for a reader and is not enforced; `properties:` names a value to extract from live traffic. The two lists are independent. See [SEP-0005](https://github.com/sightmap/sightmap/blob/main/spec/seps/0005-request-properties.md).

### Payload

```yaml
request:
  fields:
    - name: origin
      type: string
      description: IATA code
```

| Field | Type | Required | Description |
|---|---|---|---|
| `fields` | [Field](#field)[] | no | Expected fields. Not exhaustive; extra fields are not rejected. |

### Field

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | yes | The field name. |
| `type` | string | no | Free-text type label: `string`, `number`, `boolean`, `array`, `object`, or anything else an SDK author finds useful. |
| `description` | string | no | Free-text description. |

## Message

A named console-output or runtime-exception pattern. This gives console activity what `requests:` gives network activity: a named entity the rest of the corpus can point at, so "a cart version mismatch broke checkout" is stated once rather than re-matched by every consumer.

```yaml
messages:
  - name: CartVersionMismatch
    level: ERROR
    message: cart version mismatch
    description: The cart was mutated by another tab; checkout will fail.
    source: src/cart/sync.ts

  - name: SlowNetworkWarning
    level: WARN
    message: 'request .* took over \d+ms'

  - name: UncaughtCheckoutError
    level: EXCEPTION
    message: 'Cannot read propert(y|ies) .* of (null|undefined)'
```

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | yes | Stable identifier, addressable by name by other tooling. |
| `level` | string | no | Exact, case-insensitive match against the observed record's level. Match-any if omitted. |
| `message` | string | no | [RE2](#regular-expressions) regex matched against the record's text. Match-any if omitted. |
| `description` | string | no | What this pattern means, for a human reading the corpus. |
| `source` | string | no | Relative path to the source most likely to emit this. |
| `properties` | [MessageProperty](#message-properties)[] | no | Values to extract from an exception's stack. |

A record matches when every declared constraint holds. Declaring neither `level` nor `message` matches every record, which is legal but rarely useful. `message` is an [RE2 regular expression](#regular-expressions).

### Message properties

`properties:` declares named values to pull out of an uncaught exception's **stack trace**, so a corpus can classify an exception by where it came from and extract the failing location — the message-side analogue of a request's [`properties:`](#request-properties). It reuses that mechanism's `source` / `field` / `pattern` / `transform` shape.

```yaml
messages:
  - name: UncaughtCheckoutError
    level: EXCEPTION
    message: 'Cannot read propert(y|ies) .* of (null|undefined)'
    properties:
      # The throwing frame's source file and function.
      - name: origin_file
        source: stack
        field: top.file
      - name: origin_fn
        source: stack
        field: top.function
      # A specific frame by index, refined by a pattern to just the basename.
      - name: caller_base
        source: stack
        field: 1.file
        pattern: '([^/]+)$'
```

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | yes | Key a consumer refers to this value by. Must match `^[a-z][a-z0-9_]*$`. |
| `source` | string | yes | Which root to read from. The only value in v1 is `stack` (the exception's call stack). |
| `field` | string | yes | The frame and attribute to select, `<frame>.<attribute>`: `<frame>` is `top` (an alias for `0`) or a non-negative frame index (`0`, `1`, …), throwing frame first; `<attribute>` is one of `function`, `file`, `line`, or `column`. Required — a `stack` source has no meaningful bare-regex scan, the same reasoning that requires `field` for a request's headers source. |
| `pattern` | string | no | An [RE2](#regular-expressions) regex applied to whatever `field` resolved. Capture group 1 is the extracted value when the pattern has one, otherwise the entire match. |
| `transform` | string | no | Same vocabulary as [Component properties](#component-properties)' transform. |

Extraction requires **live traffic** and **value omission is silent**, exactly as for [request properties](#request-properties): a property that doesn't resolve (a plain console record with no stack, a frame index out of range, an unknown attribute, no pattern match) is simply absent, never an error. See [SEP-0006](https://github.com/sightmap/sightmap/blob/main/spec/seps/0006-message-entity.md).

### Message levels

The reference capture emits these levels:

| Level | Origin |
|---|---|
| `log`, `debug`, `info` | `console.log` / `.debug` / `.info` |
| `warn` | `console.warn` |
| `error` | `console.error`, `console.assert` |
| `exception` | An uncaught exception or unhandled rejection |

**An uncaught exception arrives as `exception`, not as `error`.** So `level: ERROR` does not match one, and a corpus that wants exceptions must say `level: EXCEPTION`. Console output and exceptions still share one entity rather than needing a `kind:` discriminator, because the origin is carried as a level value.

The vocabulary is open: `level` is free text, not an enum, so a corpus may name a level this capture never emits. That is deliberate, since another consumer's capture may have levels of its own. The tradeoff is that a typo (`level: WARNING`, which this capture normalizes to `warn`) matches nothing rather than being rejected.

### Ambiguous matches

Two entries that can match the same record are reported as `message-conflict` (a warning) when the overlap is statically decidable: the same `level`, or one omitting it, with an identical or absent `message`. Deciding whether two different regexes can both match some record is not decidable in general.

A consumer evaluating live records MUST surface an ambiguity when a record matches more than one entry, rather than silently resolving to a first match. See [SEP-0006](https://github.com/sightmap/sightmap/blob/main/spec/seps/0006-message-entity.md).

## Regular expressions

Every author-written regular expression in a sightmap — a request property's `pattern` ([Request properties](#request-properties)), a message's `message` ([Message](#message)), and a message property's `pattern` ([Message properties](#message-properties)) — uses **RE2** syntax: the dialect of Go's `regexp`, Rust's `regex`, and the `re2` npm package for JavaScript. RE2 is pinned deliberately. It matches in guaranteed linear time (no catastrophic backtracking), and because a pattern is validated at authoring time by one SDK and evaluated against live activity by another, one predictable dialect keeps the two from disagreeing about the same expression. The tradeoff is expressivity: RE2 has **no backreferences and no lookahead/lookbehind**. Character classes, alternation, quantifiers, anchors, and capture groups all work — essentially every pattern in practice.

A conforming SDK MUST reject a regular expression that is not valid RE2; the reference CLI reports `request-property-pattern-invalid` for a `pattern` and `message-regex-invalid` for a `message`.

## Memory

Memory entries are short freeform notes attached to any definition — file, view, component, or request. They exist so that agents can carry forward context that isn't recoverable from the source code: quirks, invariants, workarounds, "you have to click this twice" lore.

```yaml
memory:
  - Past dates render but are aria-disabled
  - Range: 1st click = start, 2nd = end, 3rd resets
```

Design points:

- Each entry is a single human-readable sentence or short bullet.
- Entries on a component apply whenever that component is matched on the current view.
- Entries on a view apply whenever the current URL matches that view's route.
- File-level entries apply whenever any definition from that file is active.
- Entries on a request apply in the network-trace detail view.
- Conforming SDKs SHOULD surface applicable memory entries in a `[Guide]` section at the top of enriched output.

## Route matching

Routes use glob patterns against the URL pathname.

- `*` matches exactly one path segment: `/users/*` matches `/users/42`, not `/users/42/edit`
- `**` is a globstar: **as a whole path segment** it matches zero or more segments, so `/admin/**` matches `/admin`, `/admin/users`, and `/admin/users/42/edit`, and `/a/**/b` matches `/a/b`, `/a/x/b`, …
- A `**` **glued into a segment** (e.g. `/foo**`) is treated as a regular `*` — an in-segment wildcard that does not cross a `/`. Write `**` as its own segment when you mean "any depth".
- Literal segments match themselves
- Matching is case-sensitive
- Query string and fragment are ignored
- Trailing slashes are normalized away before matching

For requests, Express-style `:param` segments are normalized to `*`. These are equivalent:

```yaml
- route: /api/users/:id/orders    # same as below
- route: /api/users/*/orders
```

### View matching: most specific wins

When multiple views could match a URL, the most specific wins. Specificity is the sum of per-segment scores:

| Segment | Score |
|---|---|
| Literal (e.g. `users`) | 3 |
| `:param` (e.g. `:id`) | 2 |
| `*` (single-segment wildcard) | 1 |
| `**` or empty | 0 |

The root route `/` scores `1` — more specific than any wildcard-only pattern. When two patterns score equal, the **first declared** view wins.

For example, given `/users/*` (score 4) and `/users/admin` (score 6), the URL `/users/admin` matches the literal — `/users/*` only wins for paths like `/users/42`.

### Request matching: all matches apply

Requests are matched independently. Every request whose `route` matches the URL — and whose optional `method` matches the request method — is applied. There is no "winning" request; all matches contribute to the enriched output.

## Global vs view-scoped

Components and requests can be declared at the file root or nested inside a view.

- **Global** (`components:` or `requests:` at file root): matched against every view.
- **View-scoped** (nested inside a `view`): matched only when that view is active.
- They are **additive**. A view that defines its own components receives both the global components and its own.

```yaml
components:
  - name: Navigation                # global — matched everywhere
    selector: 'nav[data-component="Navigation"]'

views:
  - name: Dashboard
    route: /dashboard
    components:
      - name: DashboardLayout       # scoped — only on /dashboard
        selector: '[data-component="DashboardLayout"]'
```


## Stability

Both views and components may carry an optional `stability` marker recording how much the author trusts the definition. It is advisory metadata — it does not change matching — and conforming tools SHOULD surface it (e.g. in enriched output or lint) so agents know which parts of the map are provisional.

| On | Value | Meaning |
|---|---|---|
| View | `stub` | A placeholder view, not yet fleshed out. |
| View | `deferred` | Intentionally left unmapped for now. |
| Component | `uncertain` | The selector is a best guess and unverified. |
| Component | `unstable` | Known to break across renders. |

Omit the field for an active view or a stable component.

## Tags

Views, components, and requests may all carry an optional `tags: string[]` — open-vocabulary
classification labels (e.g. `defect`) distinct from `name`. Where `name` (or a view/request's
identity) answers "what is this," `tags` answers "does this belong to some cross-cutting
classification I care about." See [SEP-0004](https://github.com/sightmap/sightmap/blob/main/spec/seps/0004-component-tags.md) for the full
proposal and rationale.

Each entity type already has a rule for resolving *identity* when more than one definition
could apply to the same match. Tags deliberately do **not** follow that rule — a broader,
tagged definition must not be shadowed by a narrower, untagged one that wins identity. Tag
resolution is instead a **union across every applicable definition**:

| Entity | Identity resolution (unchanged) | Tag resolution (this field) |
|---|---|---|
| Component | Nearest-enclosing wins — the walk from the target node toward the root stops at the first matching level. | Union across every matching ancestor level, not just the nearest. |
| View | Most-specific-route wins (see [View matching](#view-matching-most-specific-wins)). | Union across every view whose route matches the URL, not just the most-specific one. |
| Request | Not applicable — [all matching requests already apply](#request-matching-all-matches-apply); there is no single winner to begin with. | Union across every matching request — the existing "all matches apply" rule already gives this for free. |

A component example: a `CheckoutForm` tagged `defect` with an untagged `SubmitButton` child
— a click on the button resolves `name: SubmitButton` (nearest-enclosing, unchanged) and
`tags: [defect]` (inherited from the tagged ancestor).

A view example: a broad `/checkout/**` view tagged `defect`, and a more specific
`/checkout/payment` view with no tags of its own. The URL `/checkout/payment` resolves the
**view identity** `CheckoutPayment` (most-specific wins, unchanged) but still carries
`tags: [defect]` from the broader, tagged view — exactly the same shadowing concern
component tags solve, applied to route specificity instead of DOM depth.

In every case the resolved tag set MUST be deduplicated, and SHOULD be emitted in a stable
(lexicographically sorted) order wherever it is serialized. A definition that declares no
`tags` contributes nothing; this is not an error, and `tags: []` is equivalent to omitting
the field entirely.

## Reserved tooling fields

Some fields are consumed by tooling built on Sightmap (the reference CLI's capture and probe workflows) but are **not part of this spec's matching or merge semantics**. They are permitted by the schema so corpora that use them validate, but conforming SDKs MAY ignore them:

- **`access`** (on a view) — reachability of the view for a tool's reference account: `status` (`open` | `blocked` | `needs-data`) and an optional `reason`.
- **`snapshots`** (file-level) — named page states to capture (`name`, `notes`, `url`), used to enumerate capture/probe targets.

These are reserved rather than standardized: their shape may change, and other tooling need not implement them. Do not rely on them for cross-SDK matching behavior.

## Conformance

A conforming SDK:

- MUST accept any file that validates against `sightmap.schema.json`
- MUST reject any file that does not
- MUST implement route matching as specified
- MUST implement global vs view-scoped precedence as specified
- MUST implement tag resolution as a union across every applicable definition, as specified in [Tags](#tags) — never narrowed by identity-resolution rules (nearest-wins, most-specific-wins)
- MUST reject a `RequestProperty` with no `source`, or a `source` outside the four-value enum (`req.body`/`rsp.body`/`req.headers`/`rsp.headers`)
- MUST reject a `RequestProperty` that declares neither `field` nor `pattern`
- MUST reject a `RequestProperty` whose `source` is a headers source but omits `field`
- MUST reject a `RequestProperty` whose `pattern` is not a valid RE2 regular expression (see [Regular expressions](#regular-expressions))
- MUST reject a `messages:` entry whose `message` is not a valid RE2 regular expression (see [Regular expressions](#regular-expressions))
- MUST reject a `MessageProperty` with a `source` other than `stack`, or one that omits `field`, or whose `pattern` is not a valid RE2 regular expression
- SHOULD surface `memory` entries to the agent when the parent definition is active
- MAY ignore fields it doesn't use (e.g. `description` is never surfaced at runtime by Subtext today)
- MAY implement additional, non-standard behavior as long as it doesn't change the meaning of conforming inputs

An SDK that also **evaluates live activity** (observed network requests, console records, DOM state) additionally:

- MUST resolve `properties:` only from live traffic, and MUST NOT error when a `properties:`-declaring request is used in a static context — omit the value instead
- MUST omit an unresolved property value silently, without a diagnostic
- MUST apply `pattern` to the value `field` resolved (not the whole source) when both are present, taking capture group 1 as the value when the pattern has one, else the entire match
- SHOULD apply `transform` as [Component properties](#component-properties) does: skipped on an empty or absent raw value, single transform only, not composable
- MUST match a `messages:` entry by case-insensitive equality on `level` and by regex on `message`, treating either as match-any when omitted
- MUST surface an ambiguity when a record matches more than one `messages:` entry, rather than silently resolving to a first match
- MUST resolve a `MessageProperty` only from a live record's stack, omitting the value silently when the record has no stack or the addressed frame/attribute doesn't resolve

**Not yet implemented in the reference SDK.** The Go SDK under `go/` parses and validates every field above, but does not evaluate live activity: it resolves no `source`/`field`/`pattern`, applies no `transform`, and matches no `messages:` entry against a console record. The evaluation requirements in this section are normative for consumers that do evaluate, and are not yet exercised by the reference implementation or by the conformance fixtures.

## Open questions

These are explicitly unresolved in v1 and candidates for SEPs:

- Cross-sightmap (cross-project) component references — within-project is resolved by [SEP-0002](https://github.com/sightmap/sightmap/blob/main/spec/seps/0002-component-ref.md)
- Parameterized memory — interpolating runtime values into memory entries
- Schema for validating the *shape* of `response.fields` against real responses (today `fields` is documentary, not enforced)
- Macros — learned trajectories that replay and heal when the site changes (not yet in the spec)

See [`../seps/README.md`](https://github.com/sightmap/sightmap/blob/main/spec/seps/README.md) to propose.
