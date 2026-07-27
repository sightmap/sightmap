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
```

| Field | Type | Required | Description |
|---|---|---|---|
| `version` | integer | yes | Must be `1`. |
| `memory` | string[] | no | File-level memory entries. Surfaced as context to the agent for any view in this file. |
| `views` | [View](#view)[] | no | Views defined in this file. |
| `components` | (Component \| [ComponentRef](#component-references))[] | no | **Global** components — matched against every view. Entries may be either inline definitions or `$ref` reference objects. |
| `requests` | [Request](#request)[] | no | **Global** requests — matched against every view. |

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
| `description` | string | no | Free-text. Not surfaced at runtime but useful for PR review and future maintenance. |
| `source` | string | no | Relative path to the source file. |
| `dependencies` | string[] | no | Supplementary files (minimatch globs, project-root-anchored, `!` negates) whose changes should trigger re-curation of this view. See [Dependencies](#dependencies). |
| `memory` | string[] | no | View-level memory entries. |
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
| `properties` | [Property](#component-properties)[] | no | Named DOM-value extractions surfaced in enriched snapshots (e.g. `[Card price="$10"]`). Extracted from the live DOM at snapshot time; unavailable to offline tools. See [Component properties](#component-properties). |
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
- `**` matches any depth of segments: `/admin/**` matches `/admin`, `/admin/users`, `/admin/users/42/edit`
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

## Conformance

A conforming SDK:

- MUST accept any file that validates against `sightmap.schema.json`
- MUST reject any file that does not
- MUST implement route matching as specified
- MUST implement global vs view-scoped precedence as specified
- SHOULD surface `memory` entries to the agent when the parent definition is active
- MAY ignore fields it doesn't use (e.g. `description` is never surfaced at runtime by Subtext today)
- MAY implement additional, non-standard behavior as long as it doesn't change the meaning of conforming inputs

## Open questions

These are explicitly unresolved in v1 and candidates for SEPs:

- Cross-sightmap (cross-project) component references — within-project is resolved by [SEP-0002](https://github.com/sightmap/sightmap/blob/main/spec/seps/0002-component-ref.md)
- Parameterized memory — interpolating runtime values into memory entries
- Schema for validating the *shape* of `response.fields` against real responses (today `fields` is documentary, not enforced)
- Macros — learned trajectories that replay and heal when the site changes (not yet in the spec)

See [`../seps/README.md`](https://github.com/sightmap/sightmap/blob/main/spec/seps/README.md) to propose.
