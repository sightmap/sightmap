---
title: "Canonical format & diagnostics (v1)"
description: "Normative byte-level canonical YAML rules and authoring-side diagnostic codes for `.sightmap/`."
---

> **Status:** v1 **normative** spec — peer to [`schema.md`](./schema.md), conformance-tested (`spec/conformance/100-fmt-*`). On any disagreement, [`sightmap.schema.json`](./sightmap.schema.json) and this file win. See [`authoring-conventions.md`](./authoring-conventions.md) for the (informative) curation model.

## Scope

This document governs:

- Byte-level **canonical YAML output** for any tool that writes `.sightmap/*.yaml`.
- The **authoring-side diagnostic vocabulary** (`fmt.*`, `config.*`).
- The optional `.sightmap/config.yaml` version pin.

**Only agents and humans write `.sightmap/`.** The corpus is a curated authority — never generated from source, never written as a build side effect; see [`authoring-conventions.md`](./authoring-conventions.md) for the model and rationale. Any runtime consumer introspects live state and MUST NOT write `.sightmap/` files.

## Canonical formatting rules

These rules are normative: any tool writing to `.sightmap/*.yaml` MUST produce output that conforms. Conformance fixtures (`spec/conformance/100-fmt-*/`) verify byte-equivalence across implementations.

The rules constrain `ruamel.yaml` (Python) and `eemeli/yaml` (JS) tightly enough that both produce identical output for equivalent inputs.

### Indentation and style

- Two-space indentation throughout.
- Block style only for sequences and mappings — never flow style (`[a, b]` or `{k: v}`).
- One trailing newline at end of file (exactly one, no more).

### Quoting

YAML supports plain (unquoted), single-quoted, and double-quoted strings. The canonical rule, ordered by preference:

1. **Plain (unquoted)** when the string can be expressed without quoting (no leading dash, no leading colon-space, no special characters that would change parse semantics).
2. **Single-quoted** when the string contains `"` characters but no escape sequences. Single-quotes don't require escaping `"`, and this matches the natural output of both `eemeli/yaml` and `ruamel.yaml`.
3. **Double-quoted** only when the string requires escape sequences (control characters, embedded newlines, `\u` escapes, etc.).

When a string requires escape sequences, rule 3 wins regardless of `"` content; rule 2's "no escape sequences" predicate is a hard requirement, not a preference.

The selector case is the load-bearing one: `[data-sightmap="LoginButton"]` canonicalizes to `'[data-sightmap="LoginButton"]'`, not `"[data-sightmap=\"LoginButton\"]"`.

### Key ordering within entries

Explicit per entry type. Unknown keys preserved at the end in original order.

| Entry type | Canonical key order |
|---|---|
| Top-level | `version, memory, views, components, requests` |
| View | `name, route, description, source, dependencies, components, memory, requests` |
| Component | `name, selector, description, source, dependencies, children, memory` |
| Request | `name, route, method, description, source, request, response, headers, memory` |

### List ordering

- **Top-level sequences** (`views`, top-level `components`, top-level `requests`) are alphabetized: `views` and `components` by `name`, `requests` by `(route, method)` — lexicographic on the tuple, both elements compared as YAML scalar strings byte-by-byte (no Unicode normalization, no case folding). Sort keys are required and unique per schema; missing or duplicate sort keys are schema-invalid (caught by `fmt.schema-invalid` before sorting).
- **Nested sequences** (e.g. `view.components`, `component.children`) preserve insertion order. Nesting order can carry meaning (parent-child relationships, intentional declaration order); the formatter does not reorder.
- **`memory` lists** preserve insertion order. Agent-authored entries are not reordered, since order can carry meaning (recency, priority).
- **String arrays whose order is not semantically significant** — currently `dependencies` on view and component entries — are canonicalized by lexicographic sort followed by deduplication (byte-by-byte comparison, no Unicode normalization, no case folding). SDKs MUST emit `fmt.not-canonical` when an array of this kind is unsorted or contains duplicates.

### Blank lines

- One blank line between top-level sequence entries.
- No blank lines within an entry.
- One blank line after the leading comment block (if present), before content begins. The leading comment block ends at the last consecutive `#`-prefixed line; the blank line that follows is the formatter's responsibility (added if missing, normalized to exactly one), not part of the preserved comment.

### String wrapping

Long strings stay on one line. Do not wrap. YAML's wrap rules introduce ambiguity around chomping and folding that's not worth handling.

### Leading comment block

The formatter preserves a file's leading `#`-comment block verbatim — it never reflows, strips, or canonicalizes the comment text. Authors (agents or humans) may use it for a provenance note or documentation link; nothing in the format or tooling depends on its presence or absence.

### Comment-placement honesty

Round-trip preservation of comments is guaranteed when the input is already in canonical structure. When structural rewrite is required to canonicalize (key reordering, list resorting), comment positions may shift. This is a known limit, not a bug; the alternative — refusing to rewrite when comments are present — would block the formatter from doing its job.

### Validation before formatting

A formatter that operates on a `.sightmap/*.yaml` file:

- MUST refuse to rewrite a file that fails YAML parsing. Surface a `fmt.parse-error` diagnostic; leave the file untouched.
- MUST refuse to rewrite a file that fails schema validation. Surface a `fmt.schema-invalid` diagnostic; leave the file untouched.
- MUST be idempotent: a single `fmt --write` produces canonical output, and any subsequent `fmt --check` or `fmt --write` is a no-op. Comments may have moved during the first write (see *Comment-placement honesty* above); they do not move again on the second pass.

## Diagnostic codes

These are authoring-side diagnostic codes. Each has a stable `code` string, a `severity`, and a human-readable message; conforming tools surface them in their `--json` output.

### Format

| Code | Severity | Meaning |
|---|---|---|
| `fmt.not-canonical` | warning | `fmt --check` failure: file exists, parses, validates, but does not match canonical formatting. |
| `fmt.parse-error` | error | YAML parse failure during fmt. File left untouched. |
| `fmt.schema-invalid` | error | Schema validation failure during fmt. File left untouched. |

### Config

| Code | Severity | Meaning |
|---|---|---|
| `config.invalid-version` | error | `.sightmap/config.yaml` `version` field missing, malformed, or unsupported. |

### Corpus

Structural problems in a `.sightmap/` corpus. **Errors** are inputs with no valid interpretation — a missing required field, or a `$ref` that cannot be resolved. **Warnings** are conflicts where two definitions collide on identity and only one can win: the corpus still loads and a fallback rule resolves the winner (declaration order), but the author should know one definition is shadowing another.

| Code | Severity | Meaning |
|---|---|---|
| `missing-name` | error | A view or component is missing its required `name`. |
| `missing-route` | error | A view is missing its required `route`. |
| `missing-selector` | error | A component is missing its required `selector`. |
| `ref-unresolved` | error | A `$ref` names a component that no file's root `components:` defines. |
| `ref-circular` | error | A `$ref` chain is circular (e.g. `A → B → A`, or a component referencing itself). |
| `merge-collision-view` | warning | Two or more views share a `name`. Names should be unique; lookups by name and the snapshot header become ambiguous. |
| `merge-collision-component` | warning | Two or more root-level global components share a `name` with different selectors. Both match every view; resolution falls back to declaration order. |
| `route-conflict` | warning | Two or more views share the same (normalized) `route`. Only the first-declared view applies to that URL. |
| `unknown-field` | warning | A key not defined by the spec at its position (a typo like `memroy:`, or an experimental field). Warned rather than rejected so authors can stash work-in-progress fields; recognized fields — including the reserved tooling fields `access` and `snapshots` — are not flagged. |

## `.sightmap/config.yaml`

Optional in any project; pins the spec version. Schema: [`config.schema.json`](./config.schema.json). A missing, malformed, or unsupported `version` field surfaces `config.invalid-version` (see above).
