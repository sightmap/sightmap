# Sightmap spec — stream `1`

> **Status**: spec stream `1`, project semver **0.1.0** (pre-1.0). In-place tightening allowed until the project hits 1.0.0. See [`../VERSIONING.md`](../VERSIONING.md).

A sightmap is a directory of YAML files at the root of a project, under `.sightmap/`. It describes the app's **views**, **components**, and **requests**, with optional **memory** entries that carry notes agents can use at runtime.

This document is the human-readable reference. The machine-readable contract is [`sightmap.schema.json`](sightmap.schema.json).

## File discovery

- Every `*.yaml` and `*.yml` file under `.sightmap/` is discovered recursively.
- All files are loaded and merged at load time. The directory layout is a convenience for authors; it has no semantic meaning.
- Every file must begin with `version: 1`.
- Merging is shallow-append per top-level collection (`views`, `components`, `requests`). Two files may define the same view; the runtime behavior in that case is implementation-defined and SDKs SHOULD emit a warning.

## File root

```yaml
version: 1
environments: # optional, Environment[]: named deploy targets (see "Environments and origins")
origins:     # optional, map of origin name to URL: shared origins
memory:      # optional, string[] — file-level notes (see "Memory")
views:       # optional, View[]
components:  # optional, Component[] — global, matched on every view
requests:    # optional, Request[] — global, matched on every view
messages:    # optional, Message[] — console/exception patterns
```

| Field | Type | Required | Description |
|---|---|---|---|
| `version` | integer | yes | Must be `1`. |
| `environments` | [Environment](#environment)[] | no | Named deploy targets and each one's per-surface origins. Merged across files into one project-wide registry. See [Environments and origins](#environments-and-origins). |
| `origins` | map of name to [origin URL](#origin-urls) | no | Shared origins: hosts with the same URL in every environment. See [Environments and origins](#environments-and-origins). |
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
| `environments` | string[] | no | Names of the environments this view exists in. Absent or empty means every environment. See [Environments and origins](#environments-and-origins). |
| `origins` | string[] | no | Names of the origins this view is served from, resolved per environment. Absent or empty means unconstrained. |
| `url` | string | no | Representative URL for this view — a concrete address that resolves to it. Tooling uses it to navigate to the view (e.g. coverage reporting and bulk capture/probe). A file-root `url` supplies a default for every view in the file that omits its own. |
| `stability` | string | no | Authoring-confidence marker: `stub` or `deferred`. See [Stability](#stability). |
| `description` | string | no | Free-text. Not surfaced at runtime but useful for PR review and future maintenance. |
| `source` | string | no | Relative path to the source file. |
| `memory` | string[] | no | View-level memory entries. |
| `tags` | string[] | no | Open-vocabulary classification labels for this view. See [Tags](#tags). |
| `properties` | [URLProperty](#url-properties)[] | no | Named values read from the matched URL. See [URL properties](#url-properties). |
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
| `description` | string | no | Free-text. Not surfaced at runtime. |
| `memory` | string[] | no | Component-level memory entries. |
| `stability` | string | no | Authoring-confidence marker: `uncertain` or `unstable`. See [Stability](#stability). |
| `properties` | [Property](#component-properties)[] | no | Named DOM-value extractions surfaced in enriched snapshots (e.g. `[Card price="$10"]`). Extracted from the live DOM at snapshot time; unavailable to offline tools. See [Component properties](#component-properties). |
| `tags` | string[] | no | Open-vocabulary classification labels for this component. See [Tags](#tags). |
| `watch` | boolean | no | Report this component's visibility lifecycle even when it is never interacted with. See [Watch](#watch). |
| `privacy` | string | no | Whether a capture consumer may retain this element's content: `block`, `mask`, or `unmask`. See [Privacy](#privacy). |
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

See [SEP-0002](../seps/0002-component-ref.md) for the full proposal and rationale.

### Selector semantics

- A single string is a standard CSS selector.
- A list of strings is tried in order; the first selector that matches wins.
- Selectors in `children` are evaluated **within** their parent's matched subtree, not globally. This scoping is how Sightmap avoids naming collisions between, say, two different card components that both contain a `button.primary`.
- Selectors are not required to be unique at their level. If a selector matches multiple elements, all matches are named.
- **Shadow DOM.** Selectors are matched against the captured component tree, which is a *flattened* representation: each shadow root's content is inlined as ordinary children of its host. Selectors therefore match **across shadow boundaries** — shadow-DOM content is addressed exactly like light-DOM content, and there is no piercing syntax (standard CSS has none: `>>>`/`/deep/` were removed, and `::part()`/`::slotted()` reach only explicitly-exposed nodes). `children` scoping and combinators operate over the flattened tree, where a shadow host → shadow child is an ordinary parent→child edge. This is a deliberate divergence from a live `document.querySelector`, which does **not** cross shadow roots: a tool re-implementing matching against the live DOM MUST traverse shadow roots (walk each element's `shadowRoot`) to agree with the corpus.


### Component properties

A component may declare `properties: Property[]` — named values surfaced alongside the component name in enriched snapshots, e.g. `[DateFilterButton label="This Weekend"]`. Values are **resolved over the component tree**, from the matched component's own node and the extracted properties of components nested beneath it — never from arbitrary DOM. This makes resolution work offline, against a serialized tree, on any UI platform. See [SEP-0010](../seps/0010-tree-closed-component-properties.md), which supersedes the extraction model of [SEP-0003](../seps/0003-component-properties.md).

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | yes | Key used in the annotation (`name="value"`). Must match `^[a-z][a-z0-9_]*$`, and must be unique within a component. The name `value` is reserved: it may be declared to override the AX built-in, and SDKs MUST prefer a declared `value` over the AX tree's own value. |
| `extract` | [Extract](#extract) | yes | How the value is read. A component reads one of the sources below. |

**Sources.** A component property's `extract.from` is one of the following; any other value is invalid and MUST be rejected by validation.

| `from` | `path` | Resolves to |
|---|---|---|
| `dom.text` | none | the node's accessible text (implementation-defined accessible name) |
| `dom.raw_text` | none | the node's own literal text: its direct text-node children, whitespace-normalized. Excludes descendant-element text and CSS `::before`/`::after` (which are not child nodes). See [SEP-0013](../seps/0013-richer-node-data.md). |
| `dom.attr` | attribute name | the value of that attribute as carried on the node's observed attribute set; omitted if the node does not carry it |
| `dom.state` | `checked`, `selected`, `disabled` or `expanded` | the node's current interactive state, `"true"`/`"false"` (`"mixed"` for an indeterminate checkbox); omitted on a node that cannot have that state. See [Interactive state](#component-properties) and [SEP-0013](../seps/0013-richer-node-data.md). |
| `component` | `PATH.prop` | the value extracted for property `prop` of the descendant component addressed by `PATH` |
| `component.exists` | `PATH` | `"true"` if `PATH` resolves to at least one matched component; omitted otherwise (boolean state flag) |

```yaml
- name: ProductCard
  selector: '.product'
  properties:
    - name: title
      extract: { from: dom.text }
    - name: price                       # "$10.95" out of "Add to cart · $10.95"
      extract: { from: dom.raw_text, pattern: '\$([\d.]+)' }
    - name: tags                        # every Tag, joined: "sale,featured"
      extract: { from: component, path: 'Tag[].value', join: ',' }
    - name: sold_out
      extract: { from: component.exists, path: SoldOutBadge }
  children:
    - name: Tag
      selector: '.tag'
      properties:
        - name: value
          extract: { from: dom.text }
    - name: SoldOutBadge
      selector: '.sold-out'
```

`PATH` is a dotted sequence of component names naming a descendant, each segment resolved within the previous segment's matched subtree (`Price`, `Row.Price`). A plain segment takes the **first** match in document order. A segment written **`Name[]`** takes **every** match, which makes the path multi-valued: `Row[].Price.amount` reads the first `Price` in every `Row`, and `Row.Price[].amount` reads every `Price` in the first `Row`. A multi-valued path requires `join` (see [Extract](#extract)); without it the path is reserved for array-valued results and is invalid. For `from: component` the final segment of `path` is a property name; for `component.exists` the whole path names components, with no `[]`. A bracket with content in a segment (`Tab[selected=true]`) is reserved for path predicates and is invalid. References descend only — a property may address a component nested beneath the one declaring it, never a parent, sibling, or cousin — so resolution is a bottom-up pass over a DAG. To surface a value from a sub-element, promote that sub-element to a declared child component and reference it.

`[` and `]` are flow indicators in YAML, so quote a path containing `[]` inside a flow mapping: `{ from: component, path: 'Tag[].value', join: ',' }`.

The observed attribute set read by `dom.attr` is implementation-defined: which attributes a node carries depends on the consumer (a web SDK may carry a fixed allowlist plus `aria-*`/`data-*`; other platforms carry synthetic attributes). An attribute the consumer did not carry is indistinguishable from one that was absent.

**Interactive state.** Whatever else it carries, a node MUST carry its current interactive state under four names, read with `from: dom.state`: `checked`, `selected`, `disabled`, `expanded`. Each is `"true"` or `"false"` (`checked` may also be `"mixed"`) and reflects the control's *current* state, not its markup: an unchecked checkbox whose HTML carries a `checked` attribute reads `"false"`, and `<button disabled>` reads `"true"`. A name is carried only on a node that can have that state. A control's current *value* is not one of these: it remains the reserved `value` property. See [SEP-0013](../seps/0013-richer-node-data.md).

**`dom.text` vs `dom.raw_text`.** `dom.text` is the node's accessible name — what a user perceives as its label, which may weld in text from `aria-label`, an associated `<label>`, or CSS pseudo-content. `dom.raw_text` is the node's own author-written text (its direct text-node children only), computed identically on every consumer, live or offline: the deterministic escape for when the accessible name is polluted, or when the literal source text is what you want. The two are the ends of a spectrum — everything perceived, versus the literal author text. See [SEP-0013](../seps/0013-richer-node-data.md).

**`dom.attr` vs `dom.state`.** `dom.attr` reads the markup: `path: checked` is the `checked` attribute as authored, which records a control's initial state. `dom.state` reads the control's current state from the consumer's accessibility layer. They are different stores and never stand in for each other.

**Value omission is silent** — a property whose text is empty, whose attribute is not carried, whose `pattern` does not match, or whose `PATH` matches nothing is simply dropped from the annotation; consumers MUST NOT treat omission as an error.

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
| `environments` | string[] | no | Names of the environments this endpoint exists in. Absent or empty means every environment; a view-scoped request is further limited to its view's. See [Environments and origins](#environments-and-origins). |
| `origins` | string[] | no | Names of the origins this endpoint is called on, resolved per environment. Absent or empty means unconstrained. Never inherited from the enclosing view. |
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
    # A JSON body value: `path` is an object-key path within the body.
    - name: outcome
      extract: { from: rsp.body, path: status }

- name: CheckoutRetryPayment
  route: /api/checkout/pay/retry
  method: POST
  properties:
    # A header value refined by a regex: `path` names the header, `pattern`
    # extracts a substring from what it resolves to.
    - name: rate_limit_remaining
      extract: { from: rsp.headers, path: X-RateLimit-Remaining, pattern: '(\d+)' }

- name: LegacyCheckoutCallback
  route: /api/checkout/callback
  method: POST
  properties:
    # No `path`: the response is form-encoded, so there's no JSON body to
    # traverse; `pattern` scans the raw body text directly.
    - name: legacy_outcome
      extract: { from: rsp.body, pattern: '(?:declined|approved|deferred)' }
```

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | yes | Key a consumer refers to this value by. Must match `^[a-z][a-z0-9_]*$`. |
| `extract` | [Extract](#extract) | yes | How the value is read. A request reads one of the sources below. |

| `from` | `path` |
|---|---|
| `req.body`, `rsp.body` | An object-key dot-path (a numeric segment indexes an array when the value there is one — `items.0.name`). Optional when `pattern` is set, in which case the pattern scans the raw body text. |
| `req.headers`, `rsp.headers` | A header name, matched case-insensitively. **Required**: a bare regex across a raw header block is the addressing foot-gun this shape removes. |

At least one of `path`/`pattern` is required. The two compose: `path` selects a value, `pattern` optionally extracts a substring from it. `pattern` is an [RE2 regular expression](#regular-expressions); the reference CLI rejects an invalid one (`request-property-pattern-invalid`). `join` is not valid on a request property.

**Value omission is silent** — a property that doesn't resolve (a missing key, an out-of-range index, no pattern match) is simply absent; consumers MUST NOT treat omission as an error. Omission is the normal case, not an edge case: whether a body or header is even available to read depends on the capture layer's own payload and privacy settings.

Extraction requires **live traffic**. A tool operating on static corpus definitions alone MUST treat `properties:` as declared-but-unavailable, not an error.

`status`, `method`, and `duration` are **reserved identity names**, addressing the request's own already-structured HTTP identity. They sit outside `extract` entirely — a consumer may reference them wherever a property name is expected with no `properties:` declaration at all. Declaring a property under one of those names is legal and shadows the identity: the name then resolves to the extracted value, and the HTTP identity becomes unreachable. The reference CLI warns (`request-property-shadows-reserved`). Prefer a distinct name such as `outcome` unless shadowing is what you want.

A request property entry is one of two shapes, and a request may carry both side by side: a **URL-shaped** entry (`name` + `extract`) reads the request URL, described under [URL properties](#url-properties); a **payload-shaped** entry (`name` + `source` + `field`/`pattern`) reads a body or header block, described above. One entry cannot mix the two.

`properties:` and `request:`/`response:` (Payload) answer different questions: `Payload.fields[]` documents expected shape for a reader and is not enforced; `properties:` names a value to extract from live traffic. The two lists are independent. See [SEP-0005](../seps/0005-request-properties.md).

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
| `tags` | string[] | no | Open-vocabulary classification labels for this message. See [Tags](#tags). |
| `properties` | [MessageProperty](#message-properties)[] | no | Values to extract from an exception's stack. |

A record matches when every declared constraint holds. Declaring neither `level` nor `message` matches every record, which is legal but rarely useful. `message` is an [RE2 regular expression](#regular-expressions).

### Message properties

`properties:` declares named values to pull out of an uncaught exception's **stack trace**, so a corpus can classify an exception by where it came from and extract the failing location — the message-side analogue of a request's [`properties:`](#request-properties), using the same [Extract](#extract) object.

```yaml
messages:
  - name: UncaughtCheckoutError
    level: EXCEPTION
    message: 'Cannot read propert(y|ies) .* of (null|undefined)'
    properties:
      # The throwing frame's source file and function.
      - name: origin_file
        extract: { from: stack, path: top.file }
      - name: origin_fn
        extract: { from: stack, path: top.function }
      # A specific frame by index, refined by a pattern to just the basename.
      - name: caller_base
        extract: { from: stack, path: 1.file, pattern: '([^/]+)$' }
```

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | yes | Key a consumer refers to this value by. Must match `^[a-z][a-z0-9_]*$`. |
| `extract` | [Extract](#extract) | yes | How the value is read. The only source is `stack`, the exception's call stack. |

`path` is required and addresses a frame and attribute, `<frame>.<attribute>`: `<frame>` is `top` (an alias for `0`) or a non-negative frame index (`0`, `1`, …), throwing frame first; `<attribute>` is one of `function`, `file`, `line`, or `column`. A `stack` source has no meaningful bare-regex scan, the same reasoning that requires `path` for a request's headers source. `join` is not valid on a message property.

Extraction requires **live traffic** and **value omission is silent**, exactly as for [request properties](#request-properties): a property that doesn't resolve (a plain console record with no stack, a frame index out of range, an unknown attribute, no pattern match) is simply absent, never an error. See [SEP-0006](../seps/0006-message-entity.md).

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

A consumer evaluating live records MUST surface an ambiguity when a record matches more than one entry, rather than silently resolving to a first match. See [SEP-0006](../seps/0006-message-entity.md).

## Signal

A named, reference-based **state predicate**: a signal names an existing component or view (`ref:`) and denotes the boolean of that entity's *current* state — a component being present, or a view's route being active. It is the smallest, dependency-free slice of [SEP-0007](../seps/0007-signals.md): the point-signal shape restricted to `Component` and `View` refs (a component ref leans only on the existing [component properties](#component-properties); a view ref needs nothing). Request and message refs — and the temporal/window machinery of the fuller signals proposal — are intentionally out of scope in this subset.

Its purpose is to give the rest of the tooling a named boolean to point at: a completion predicate ("done once `checkout.reached` holds"), an availability predicate ("offer the dismiss affordance while `upsell.present` holds"), or a session classification. Because a component ref evaluates exactly like a component match and a view ref like a route match, a signal is the *named* form of a predicate the matcher already computes.

```yaml
signals:
  - name: checkout.reached
    ref: Checkout           # a view -> its route is active

  - name: upsell.present
    ref: UpsellModal        # a component -> it currently matches (is present)
    tags: [interstitial]
```

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | yes | Semantic identity of the signal — a named boolean predicate, addressable by other tooling. |
| `ref` | string | yes | Name of an existing component or view this signal is about. Must resolve to exactly one. |
| `tags` | string[] | no | Open-vocabulary classification labels carried onto the signal. |

`ref` must resolve to exactly one entity. A name that matches nothing is reported as `signal-ref-unresolved`; a name that matches **both** a component and a view is `signal-ref-ambiguous` (there is no adjacency rule to prefer one, so it is rejected rather than silently resolved). Signal names must be unique across the corpus. `signals:` is corpus-root only — there is no view-scoped form. See [SEP-0007](../seps/0007-signals.md).

## Extract

Every property that extracts a value (component, request, message) declares how with one object. See [SEP-0017](../seps/0017-extract-object.md).

| Field | Type | Required | Description |
|---|---|---|---|
| `from` | string | yes | The source to read from. Which sources are valid depends on the entity: see [Component properties](#component-properties), [Request properties](#request-properties), [Message properties](#message-properties). `url.query` and `url.path` read the matched URL, on views and requests (see [URL properties](#url-properties)). |
| `path` | string | per source | The value within the source. Required or forbidden depending on `from`. |
| `pattern` | string | no | An [RE2](#regular-expressions) regex applied to the resolved value. Capture group 1 is the value when the pattern has one, otherwise the entire match. A value the pattern does not match is omitted. Not valid with `component.exists`. |
| `join` | string | no | Valid only with `from: component`, and required by a multi-valued path (one with a `Name[]` segment), which it collapses into one value: each value is read and refined by `pattern`, empty values are dropped, and the rest are joined with this string. No surviving value omits the property. Invalid on a single-valued path. Must be non-empty. |

No other keys are allowed.

**Deprecated string forms.** Earlier versions of this spec spelled extraction per entity. Those forms remain valid during a deprecation window, are lowered exactly to the object, and draw an `extract-legacy-form` warning. They will be removed in a later release.

| Deprecated | Object |
|---|---|
| `extract: text` | `{ from: dom.text }` |
| `extract: raw_text` | `{ from: dom.raw_text }` |
| `extract: attr=NAME` | `{ from: dom.attr, path: NAME }` |
| `extract: PATH.prop` | `{ from: component, path: PATH.prop }` |
| `extract: exists:PATH` | `{ from: component.exists, path: PATH }` |
| `source: S`, `field: F`, `pattern: P` (request, message) | `{ from: S, path: F, pattern: P }` |

`attr=NAME` always lowers to `dom.attr`, including for the interactive-state names. A property may not combine `extract` with `source`, `field` or `pattern` (`extract-shape-mixed`).

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

## URL properties

A `:name` segment in a view or request `route` **binds** that segment's percent-decoded value as a property named `name`. An optional `properties[]` array names query-string values and renames bound segments, using the [Extract](#extract) object with `from: url.query` or `from: url.path`. The same grammar applies to both entities: both carry a `route`, both are matched against a URL, and the value an author wants out of that URL is the same kind of thing in each case.

```yaml
views:
  - name: ProductDetail
    route: /shop/p/:product_id        # binds product_id
    properties:
      - name: variant
        extract: { from: url.query, path: variant }
requests:
  - name: GetOrder
    route: /api/orders/:order_id      # binds order_id
    method: GET
    properties:
      - name: variant
        extract: { from: url.query, path: variant }   # reads the URL
      - name: outcome
        extract: { from: rsp.body, path: status }     # reads the body, see Request properties
```

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | yes | Key a consumer refers to this value by. Must match `^[a-z][a-z0-9_]*$`. Must not equal a `:name` binding the entity still produces (see below). |
| `extract` | [Extract](#extract) | yes | `from: url.path` with `path: <segment_name>` reads a `:name` bound by this entity's own route; `from: url.query` with `path: <key>` reads a query-string parameter. `path` is required; `pattern` refines the value; `join` is not valid. |

Given `route: /shop/p/:product_id` and a property `{ name: variant, extract: { from: url.query, path: variant } }`:

| URL | Matches? | Properties |
|---|---|---|
| `/shop/p/12345` | yes | `product_id="12345"` |
| `/shop/p/12345?variant=blue` | yes | `product_id="12345"`, `variant="blue"` |
| `/shop/p/12345?VARIANT=blue` | yes | `product_id="12345"` (keys are case-sensitive) |
| `/shop/p/12345?variant=` | yes | `product_id="12345"` (empty resolves to nothing) |
| `/shop/p/12345?variant=a&variant=b` | yes | `product_id="12345"`, `variant="a"` (first occurrence) |
| `/shop/p/blue%20suede` | yes | `product_id="blue suede"` (percent-decoded) |
| `/shop/p/12345#reviews` | yes | `product_id="12345"` (the fragment is ignored) |
| `/shop/p/12345/reviews` | no | `:product_id` binds exactly one segment |
| `/shop/p/` | no | a `:name` requires a segment to be present |

`url.query` keys are matched **case-sensitively** — URLs are case-sensitive below the host — and resolve to the first occurrence, percent-decoded.

**Binding is implicit**: a `:name` needs no `properties[]` entry, the same way a request's reserved identity names do not. An entry reading `from: url.path` renames that segment: its value arrives under the entry's name and the implicit `:name` is no longer produced. Every name an entity produces must be unique, so a declared property, of any source, named like a binding the entity still produces is an error (`route-binding-conflict`); rename the binding first to free its name. A binding name must match `^[a-z][a-z0-9_]*$`: a `:segment` that does not (`:orgId`) still matches but binds nothing, and the reference CLI warns (`route-param-unbound`). A `:name` MUST NOT repeat within one route, and on a request a bound name colliding with a reserved identity name (`status`, `method`, `duration`) is an error rather than a silent precedence rule.

**This changes what `:param` produces, never what it matches.** [Route matching](#route-matching) still normalizes `:param` to `*` for requests, and a view `:param` still scores specificity `2`, so every existing route matches exactly the URL set it matched before. `**` cannot bind: it spans a variable number of segments, so there is no single value to name.

**These resolve statically.** Both sources read a URL string rather than live DOM state or live traffic, so a saved session, a coverage report, or a lint pass over a URL alone produces every declared URL property with no live observation.

**A declaration is also a retention request.** A consumer that scrubs captured URLs SHOULD retain the parts its corpus names, and SHOULD surface the case where it dropped one anyway — a scrubbed parameter makes the property permanently unresolvable, and omission is otherwise silent. Naming a part is **not** a privacy grant: a consumer MUST NOT treat a declaration as authorization to retain a value its own policy, or [`privacy`](#privacy), would withhold. A URL property also promotes whatever the parameter holds into a named value that flows onward, so naming a parameter carrying personal data spreads it rather than containing it.

See [SEP-0008](../seps/0008-url-properties.md).

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

## Environments and origins

A corpus can say *where* its views and requests run, not only what they are. A file-root `environments` array defines named **deploy targets** (`staging`, `prod`, `android-beta`), and a file-root `origins` map defines **shared origins**: hosts that are the same in every deploy target, like a tracking pixel or a vendor endpoint. Views and requests then reference both by name. See [SEP-0014](../seps/0014-environments-and-origins.md) for the full proposal and rationale.

```yaml
version: 1
environments:
  - name: android-prod
    platform: android
    app_id: com.acme.app
    build_type: release
    backend: prod
  - name: preview
    origins:
      api: https://api.staging.acme.com
      app: https://deploy-preview-*--acme.netlify.app
  - name: prod
    origins:
      api: https://api.acme.com
      app: https://app.acme.com
  - name: staging
    origins:
      api: https://api.staging.acme.com
      app: https://app.staging.acme.com

origins:
  facebook: https://www.facebook.com

views:
  - name: OrderHistory
    route: /orders/history
    environments: [preview, prod, staging]
    origins: [app]
    requests:
      - name: ListOrders
        route: /orders
        method: GET
        origins: [api]
      - name: FacebookPixel
        route: /tr
        method: GET
        origins: [facebook]
```

An environment answers "which deploy target"; an origin answers "which host". A deploy target is what a session belongs to and what a corpus is published to. A **surface** (`app`, `api`) is a host a deploy target serves pages from or calls, so `api` is one name and each environment says what its API host is.

### Environment

| Field | Type | Applies to | Required | Description |
|---|---|---|---|---|
| `name` | string | all | yes | Matches `^[a-z][a-z0-9_-]*$`. Unique among environments. |
| `platform` | `web` \| `ios` \| `android` | all | no, default `web` | Which kind of deploy target this is. |
| `origins` | map of name to [origin URL](#origin-urls) | all | web: yes; native: no | Each surface's URL in this environment. On a native environment it overrides the `backend`'s entry of the same name. |
| `app_id` | string | native | yes | The bundle ID (iOS) or application ID (Android) the app reports: dot-separated segments of `[A-Za-z0-9_-]`. |
| `build_type` | string | native | no | The build variant or configuration name the app reports, matched verbatim. Separates environments that share one `app_id`. |
| `backend` | string | native | no | The name of a **web** environment whose origins this environment borrows. |

A web environment (`platform` absent or `web`) is identified by its origins, MUST define `origins`, and MUST NOT set `app_id`, `build_type`, or `backend`. A native environment (`ios` or `android`) is identified by its `app_id` and optional `build_type`; it has no page hosts of its own and borrows a web environment's origins through `backend`. `backend` MUST name a web environment, so backends never chain.

### Origin URLs

Origin names match `^[a-z][a-z0-9_-]*$`. An origin URL is `scheme://host[:port]` with an `http` or `https` scheme and no path, query, or fragment. The port, when present, is 1 to 65535.

An origin URL can be a **pattern** naming a set of origins, with wildcards in exactly three positions:

- **`*` inside the leftmost host label** matches one or more characters within that label: `https://deploy-preview-*--acme.netlify.app`, `https://*.preview.acme.com`.
- **`**` as the whole leftmost host label** matches one or more labels: `https://**.acme.com` matches `app.acme.com` and `app.staging.acme.com`.
- **`*` as the port** matches any port, or none: `http://localhost:*`.

The scheme never carries a wildcard, and a host pattern never matches its own bare suffix (`https://*.acme.com` does not match `https://acme.com`). A pattern is something to match against, not an address: a consumer can test a session's origin against it or anchor a request matcher with it, but cannot navigate to it.

### Registries and references

Environment definitions from every loaded file form one project-wide registry, and shared origins another, with the same lookup scope [component references](#component-references) use. Two environments sharing a `name`, or two shared origins sharing a name, collide: the one from the first file by source path wins.

A view's or request's `environments` and `origins` hold bare names, never definitions; a file root holds definitions, never bare names. Every environment reference MUST name a defined environment, and every origin reference MUST name an origin defined in some environment's `origins` or in the shared map.

**Absent or empty means unconstrained.** A view or request with no `environments` exists in every environment; with no `origins`, it may be served from any origin its environment defines. An explicit `[]` means the same as an absent list, so an accidentally emptied list does not flip an entity from matching everywhere to matching nothing.

**View-scoped requests.** A view-scoped request exists only where its view exists: its effective environments are its own list intersected with its view's, where an absent list on either side contributes no constraint. `origins` are **not** inherited, because a page and the endpoints it calls routinely live on different hosts.

### Origin resolution

An origin name resolves, for a given environment, by checking in order:

1. the environment's own `origins`;
2. for a native environment, its `backend`'s `origins`;
3. the shared `origins` map.

The first match is the URL; with no match, the name resolves to nothing in that environment. Each step overrides the ones below it, so any environment can override a shared entry, and a native build can point one host somewhere other than its backend's.

### Session membership

A **web** session belongs to the web environment with the most specific origin matching the session's origin. Shared origins and native environments' origins never identify a session. Specificity follows route matching's rule:

1. A literal origin beats any pattern.
2. Between patterns, compare host labels from the right. At the first label where they differ, a literal label beats a label containing `*`, which beats a bare `*`, which beats `**`. A pattern with more labels beats one with fewer when every shared label is equally specific.
3. A literal port beats `:*`.

If the most specific matches belong to more than one web environment, the session belongs to none of them. Matching compares host and port only, not scheme, after lowercasing the host and dropping a port of 80 or 443. A consumer that cannot observe a session's port compares the host alone and resolves to nothing when that is ambiguous.

A **native** session belongs to the environment whose `platform` and `app_id` match its app, preferring one whose `build_type` matches the session's build variant verbatim; an environment with no `build_type` covers every variant of its `app_id`. App IDs compare byte-for-byte. Where a platform reports no build variant, a declared `build_type` matches nothing, so declare at most one environment per `app_id` on such a platform.

### Not a route-matching input

A URL matches a view or request by path alone, exactly as in [Route matching](#route-matching). A session outside an entity's environments, or a URL outside its origins, still matches that entity's route. Environments and origins are data about an entity for consumers to use as they need: choosing a publish target, compiling host-scoped page definitions per environment, anchoring request matchers.

`url:` is complementary. It is one concrete, navigable address and implies neither an environment nor an origin; a consumer MUST NOT derive either from it. A consumer that needs a host for a view that resolves no origin MAY fall back to the `url:` host, and never does once the view resolves one.

### Diagnostics

| Code | Severity | Meaning |
|---|---|---|
| `environment-invalid` | error | An environment breaks the web/native shape rules: a web environment without `origins` or with `app_id`, `build_type`, or `backend`; a native one without a valid `app_id`; or an invalid `name` or `platform`. |
| `origin-invalid` | error | An origin name or URL breaks the [origin URL](#origin-urls) grammar, including a port above 65535. |
| `environment-backend-invalid` | error | A `backend` names no environment, or names a native one. |
| `environment-ref-unresolved` | error | A view or request names an environment that is not defined. |
| `origin-ref-unresolved` | error | A view or request names an origin that no environment and no shared map defines. |
| `environment-name-collision` | warning | Two environments share a `name`; the first by source-file path wins. |
| `origin-name-collision` | warning | Two shared origins share a name; the first by source-file path wins. |
| `origin-environment-gap` | warning | An origin name resolves in some web environments but not others. A name in the shared map resolves everywhere and never produces this. |
| `origin-host-shared` | warning | Two web environments define the same origin, compared on host and port. Sharing alone is legal, but that host identifies no session; a host several environments call belongs in the shared map. |
| `environment-duplicate` | warning | Two native environments share `platform`, `app_id`, and `build_type`: the same target under two names. |
| `environments-empty`, `origins-empty` | warning | A view or request declares an explicit empty list, which means the same as omitting it. |

A corpus that declares neither field anywhere produces none of these.

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

Views, components, requests, messages, and signals may all carry an optional `tags: string[]` — open-vocabulary
classification labels (e.g. `defect`) distinct from `name`. Where `name` (or a view/request's
identity) answers "what is this," `tags` answers "does this belong to some cross-cutting
classification I care about." See [SEP-0004](../seps/0004-component-tags.md) for the full
proposal and rationale, and [SEP-0016](../seps/0016-message-and-signal-tags.md) for messages and
signals.

Each entity type already has a rule for resolving *identity* when more than one definition
could apply to the same match. Tags deliberately do **not** follow that rule — a broader,
tagged definition must not be shadowed by a narrower, untagged one that wins identity. Tag
resolution is instead a **union across every applicable definition**:

| Entity | Identity resolution (unchanged) | Tag resolution (this field) |
|---|---|---|
| Component | Nearest-enclosing wins — the walk from the target node toward the root stops at the first matching level. | Union across every matching ancestor level, not just the nearest. |
| View | Most-specific-route wins (see [View matching](#view-matching-most-specific-wins)). | Union across every view whose route matches the URL, not just the most-specific one. |
| Request | Not applicable — [all matching requests already apply](#request-matching-all-matches-apply); there is no single winner to begin with. | Union across every matching request — the existing "all matches apply" rule already gives this for free. |
| Message | Ambiguous matches are **refused**: a consumer MUST surface an ambiguity rather than pick a winner. | Union across every matching entry. The classification therefore survives an ambiguity the identity does not: a consumer that cannot say *which* message a record is can still say it is tagged `defect`. |
| Signal | Single, named classification; `ref` resolves to exactly one entity. | Union of the signal's own `tags` with the resolved tags of the entity named by `ref`. |

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

## Watch

A component may declare `watch: true`, asking a capture consumer to report its **visibility lifecycle** — that it rendered, that it became visible, that it went away — rather than reporting it only when someone interacts with it.

```yaml
- name: NoResultsMessage
  selector: '.search-empty'
  watch: true
```

For each matched element, a consumer that reports visibility MUST report that the element **became visible to the user**, and MAY report the rest of the lifecycle around it — rendered, no longer visible, removed. Becoming visible is the only moment that answers the question the field exists for; the surrounding events are largely churn, since an element can render far off-screen, re-render on every state change, and be removed by a route transition. A consumer whose only notion is a coarse "seen" conforms with that alone.

Repeat reports SHOULD be collapsed: an element that leaves and re-enters the viewport, or whose observer fires several times for one appearance, is one appearance. Each matched element is reported separately; a consumer that cannot distinguish instances MUST still report the first.

**Visibility is passive.** An element scrolling into view is a layout side effect, not something the user did, so a consumer that ranks or attributes activity MUST NOT treat a visibility report as an interaction.

**`watch` applies to the component it is declared on, never to its `children`.** This is the opposite of [`privacy`](#privacy), deliberately: privacy is a restriction, where covering the subtree is the safe default, while `watch` generates records, where covering a subtree silently would multiply them.

**A consumer's interactivity heuristic MUST NOT suppress a watched component.** A consumer that would otherwise skip an element because it is not interactive MUST report a watched one anyway — without this, the field does nothing for exactly the non-interactive components that motivate it. Other filters a consumer applies for its own correctness are unaffected, but it SHOULD surface that it dropped the request rather than ignoring it.

`watch: false` is identical to omitting the field. `watch` takes no part in route matching, component identity, or specificity.

`watch` and [`privacy`](#privacy) are independent and compose without special rules: a component may be watched and blocked at once, reporting that it appeared while retaining none of its content.

See [SEP-0015](../seps/0015-component-watch.md).
## Privacy

A component may declare `privacy`, stating whether a capture consumer — anything that records the page for later replay or analysis — may retain the matched element's content.

```yaml
- name: CheckoutForm
  selector: '.checkout-form'
  privacy: mask
  children:
    - name: CardNumberInput
      selector: 'input[name="cc"]'
      privacy: block
    - name: OrderTotal
      selector: '.order-total'
      privacy: unmask
```

| Value | Meaning |
|---|---|
| `block` | The element and its subtree MUST NOT be captured. Neither content nor structure is retained. |
| `mask` | The element's structure and layout MAY be captured; its text, input values, and attribute values MUST NOT be, except the interactive-state attributes noted below. |
| `unmask` | The element and its subtree are captured in full, overriding any enclosing `block` or `mask`. |

A declaration applies to the matched element **and its entire subtree**, so masking a form masks the fields inside it without naming each one.

**Resolution is nearest-enclosing wins** — the same rule component identity follows, and deliberately *not* the union rule [`tags`](#tags) uses. A union is right for classification, where more labels are additive; it is wrong for a directive, where two applicable values are a contradiction that must be decided rather than merged. This is also what makes `unmask` meaningful: it is inert in isolation and exists to carve one safe element out of a broader restriction.

**Omission declares nothing.** A component with no `privacy` field makes no statement, and the consumer's own default is unchanged. A corpus can adopt the field one component at a time without implying anything about the rest.

**The corpus is a floor, not a ceiling.** A consumer MAY withhold more than the corpus asks. A consumer MUST NOT capture content the resolved value marks `block` or `mask`. An `unmask` states that the corpus author considers the element safe; it does not override a consumer's own policy, and a consumer that blocks the element for its own reasons MUST continue to.

**Extracted properties are governed too**, at the node the value is read from. A [`properties[]`](#component-properties) entry produces a named value that travels separately from captured content, so a directive covering only the recording would leak the same text through the other path. A property whose value resolves from a node whose effective privacy is `block` or `mask` MUST NOT be surfaced.

The governing node is the one the value is **read from**, not the component that declared the property. Extraction is tree-closed, so a `from: component` read resolves against a descendant component and that descendant's own resolved privacy applies — a component with no declaration of its own cannot launder a value out of a blocked descendant.

`component.exists` is an exception, reporting presence rather than content: it MAY be surfaced under `mask`, which already permits structure, and MUST NOT when the target is `block`. `dom.text`, `dom.raw_text` and `dom.attr` yield content and are withheld under either, except a `dom.attr` read of an interactive-state attribute under `mask` (below). `dom.state` carries state rather than a value and MAY be surfaced under `mask`; under `block` nothing resolves.

A property that reads `dom.text`, `dom.raw_text` or `dom.attr` from a component whose own `privacy` is `block` or `mask` (a `dom.attr` read of an interactive-state attribute only under `block`), or `dom.state` from one whose own `privacy` is `block`, is withheld by every capture consumer, so the reference CLI warns (`extract-privacy-withheld`). The remedy is to change the component's privacy, where a privacy review sees it.

**Attributes under `mask`.** An attribute can be structure or content, so the line is drawn here rather than left to each consumer. Under `mask` a consumer MUST withhold the value of any attribute the corpus reads via a `dom.attr` extract **anywhere in the corpus** — naming an attribute in an extract is a declaration that it carries a value worth reading — and the value of any `data-*` attribute, and of `value`, `title`, `alt`, `placeholder` and `aria-label`. A consumer MAY retain the four interactive-state attributes [SEP-0013](../seps/0013-richer-node-data.md) defines (`checked`, `selected`, `disabled`, `expanded`), which are a closed set carrying state rather than a value, and presentational attributes needed to render the element's shape (`class`, `style`, `id`). A `dom.attr` read of one of those four names resolves under `mask`; every other `dom.attr` read is withheld. Under `block` nothing resolves.

A `class` or `id` built from user data survives a `mask`, and the spec cannot close that without making `mask` unimplementable for replay. An author carrying user data in a presentational attribute should use `block`.

A withheld property is **absent**, exactly as if it had not resolved, so [silent value omission](#component-properties) covers it and a consumer cannot distinguish the two cases.

`privacy` takes no part in route matching, component identity, or specificity. Two components differing only in `privacy` are the same component for every other purpose. A consumer that does not capture page content MUST accept and ignore the field.

See [SEP-0009](../seps/0009-component-privacy.md).

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
- MUST reject an [Extract](#extract) whose `from` is not valid on its entity, whose `path` is missing where its source requires one or present where its source forbids one, whose `pattern` is not a valid RE2 regular expression (see [Regular expressions](#regular-expressions)), or that carries `join` with any source but `component`
- MUST reject a `RequestProperty` that declares neither `path` nor `pattern`, or reads a headers source without `path`
- MUST accept the deprecated extract string forms during the deprecation window, lowered exactly as [Extract](#extract) tables, and SHOULD warn on each
- MUST reject a `messages:` entry whose `message` is not a valid RE2 regular expression (see [Regular expressions](#regular-expressions))
- MUST reject a `MessageProperty` that reads any source but `stack`, or omits `path`
- MUST reject an environment that breaks the web/native shape rules, an invalid origin URL, a `backend` that names no environment or a native one, and an environment or origin reference that resolves to no definition (see [Environments and origins](#environments-and-origins))
- MUST build one project-wide environment registry and one shared-origin registry, the first by source-file path winning on a name collision, and MUST resolve an origin for an environment from its own `origins`, then its `backend`'s, then the shared map
- MUST treat an absent or empty view- or request-level `environments`/`origins` list as unconstrained, MUST intersect a view-scoped request's environments with its view's, and MUST NOT inherit a view's `origins` into its requests
- MUST NOT treat `environments` or `origins` as an input to route matching, and MUST NOT derive either from `url:`; MAY use the `url:` host for a view that resolves no origin
- SHOULD surface `memory` entries to the agent when the parent definition is active
- MAY ignore fields it doesn't use (e.g. a consumer that never surfaces `description` at runtime)
- MAY implement additional, non-standard behavior as long as it doesn't change the meaning of conforming inputs

An SDK that also **evaluates live activity** (observed network requests, console records, DOM state) additionally:

- MUST resolve `properties:` only from live traffic, and MUST NOT error when a `properties:`-declaring request is used in a static context — omit the value instead
- MUST omit an unresolved property value silently, without a diagnostic
- MUST apply `pattern` to the value `path` resolved (not the whole source) when both are present, taking capture group 1 as the value when the pattern has one, else the entire match
- MUST match a `messages:` entry by case-insensitive equality on `level` and by regex on `message`, treating either as match-any when omitted
- MUST surface an ambiguity when a record matches more than one `messages:` entry, rather than silently resolving to a first match
- MUST resolve a `MessageProperty` only from a live record's stack, omitting the value silently when the record has no stack or the addressed frame/attribute doesn't resolve
- MUST assign a session to an environment as specified in [Session membership](#session-membership): a web session to the web environment with the most specific matching origin (and to none when the most specific matches span several), comparing host and port only; a native session by `platform` and `app_id`, matching `build_type` verbatim and treating an environment with no `build_type` as covering every variant

**Not yet implemented in the reference SDK.** The Go SDK under `go/` parses and validates every field above, and resolves request and message properties and `messages:` matches against a record handed to it, but observes no live traffic itself and assigns no session to an environment. The evaluation requirements in this section are normative for consumers that do evaluate, and are not yet exercised by the reference implementation or by the conformance fixtures.

## Open questions

These are explicitly unresolved in v1 and candidates for SEPs:

- Cross-sightmap (cross-project) component references — within-project is resolved by [SEP-0002](../seps/0002-component-ref.md)
- Parameterized memory — interpolating runtime values into memory entries
- Schema for validating the *shape* of `response.fields` against real responses (today `fields` is documentary, not enforced)
- Macros — learned trajectories that replay and heal when the site changes (not yet in the spec)

See [`../seps/README.md`](../seps/README.md) to propose.
