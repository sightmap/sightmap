# Conformance fixtures

Language-agnostic test cases. Every Sightmap SDK port is expected to pass these.

## Layout

Each fixture is a directory named `NNN-{slug}.fixture/` (three-digit number, kebab-case slug, `.fixture` suffix — see [`CONVENTIONS.md`](../CONVENTIONS.md)) containing:

- `sightmap/` — input YAML files (the simulated `.sightmap/` directory)
- `expected.json` — list of `{ command, args, expected }` test cases the runner checks

## Test case shape

```json
{
  "cases": [
    {
      "command": "validate" | "match" | "explain" | "lint" | "fmt",
      "args": { "...": "command-specific" },
      "expected": { "...": "subset of the command's JSON output to assert" }
    }
  ]
}
```

The runner asserts that every key in `expected` is present in the actual output and matches deeply. Extra keys in the actual output are allowed (forward-compatible).

For arrays in `expected`, the actual array must be at least as long, and the prefix must match.

## Adding a fixture

1. Create the next-numbered directory under `conformance/`, named `NNN-{slug}.fixture/`.
2. Author `sightmap/*.yaml` and `expected.json`.
3. Run the conformance runner from any SDK to verify.
4. Open a PR.

## Current fixtures

| # | Name | Exercises |
|---|---|---|
| 001 | `minimal` | Smallest valid sightmap; basic `match` |
| 002 | `multi-file-merge` | Same view name across two files → `merge-collision-view` warning |
| 003 | `route-precedence` | Most-specific-wins: literal &gt; `:param` &gt; `*` &gt; `**`, declaration order tie-breaks |
| 004 | `param-normalization` | Express-style `:param` normalizes to `*` (requests); `:param` also matches a single view-route segment |
| 005 | `selector-array` | `selector` accepts array; alternates tried in order |
| 006 | `view-scoped-vs-global` | Global components match everywhere; scoped only on their view |
| 007 | `request-method-filter` | `match` filters requests by HTTP method |
| 010 | `component-ref` | `$ref` expansion, view attestation, and global+view-scoped dedup ([SEP-0002](../seps/0002-component-ref.md)) |
| 011 | `component-ref-unresolved` | `$ref` to an unknown component → `ref-unresolved` error |
| 012 | `component-ref-circular` | Self-referential `$ref` chain → `ref-circular` error |
| 013 | `route-trailing-slash` | Trailing slashes on the URL path are normalized away before matching |
| 014 | `component-properties` | `properties:` with the tree-closed `extract` object (`dom.text`, `component` `PATH.prop`, `component.exists`) validates ([SEP-0010](../seps/0010-tree-closed-component-properties.md), [SEP-0017](../seps/0017-extract-object.md)) |
| 015 | `view-url` | `url:` on a view (and a file-level default) validates |
| 016 | `stability-tooling-fields` | `stability:` (view + component) validates; reserved tooling fields `access:`/`snapshots:` are permitted |
| 017 | `tags` | `tags:` validates on components (at multiple nesting levels), requests, views, messages, and signals ([SEP-0004](../seps/0004-component-tags.md), [SEP-0016](../seps/0016-message-and-signal-tags.md)) |
| 018 | `request-properties` | `properties:` on a request validates via `extract` (body and header `path`s, and `pattern`); declaring a reserved identity name warns with `request-property-shadows-reserved` ([SEP-0005](../seps/0005-request-properties.md)) |
| 019 | `messages` | `messages:` validates with `level`/`message`/`description`/`source`, including `level: EXCEPTION`; a level-only entry overlapping a level+message entry warns with `message-conflict` ([SEP-0006](../seps/0006-message-entity.md)) |
| 020 | `message-properties` | `properties:` on a message validates via `extract: { from: stack }` with a `<frame>.<attribute>` `path` and an optional `pattern` ([SEP-0006](../seps/0006-message-entity.md)) |
| 021 | `component-privacy` | `privacy:` validates on components at every nesting depth, global and view-scoped, with `block`/`mask`/`unmask` nested under one another ([SEP-0009](../seps/0009-component-privacy.md)) |
| 022 | `component-watch` | `watch:` validates on components at every nesting depth, global and view-scoped, including an explicit `false` ([SEP-0015](../seps/0015-component-watch.md)) |
| 023 | `url-properties` | `:name` route segments bind on views and requests with no `properties:` entry; `from: url.query`/`url.path` validates on both, and a request carries URL and payload sources side by side ([SEP-0008](../seps/0008-url-properties.md)) |
| 024 | `message-tags-ambiguity` | Tags resolve as a union across every matching `messages:` entry even where identity is ambiguous, so `message-conflict` warns while the tag union still holds ([SEP-0016](../seps/0016-message-and-signal-tags.md)) |
| 025 | `environments-and-origins` | File-root `environments` (web with literal and pattern origins, native with `app_id`/`build_type`/`backend`, `platform` defaulted) and shared `origins`, referenced across files by views and requests; two web environments sharing an API host warns with `origin-host-shared` ([SEP-0014](../seps/0014-environments-and-origins.md)) |
| 026 | `environment-refs-unresolved` | A view or request (global or view-scoped) naming an undefined environment → `environment-ref-unresolved`; an origin defined nowhere → `origin-ref-unresolved` ([SEP-0014](../seps/0014-environments-and-origins.md)) |
| 027 | `environment-definitions-invalid` | Schema-valid definitions only the validator can reject: a `backend` naming no environment or a native one → `environment-backend-invalid`; a port above 65535 → `origin-invalid` ([SEP-0014](../seps/0014-environments-and-origins.md)) |
| 028 | `environment-registry-warnings` | First-by-path wins across files (`environment-name-collision`, `origin-name-collision`), `environment-duplicate`, `origin-environment-gap`, and explicit empty reference lists (`environments-empty`, `origins-empty`) ([SEP-0014](../seps/0014-environments-and-origins.md)) |
| 029 | `extract-legacy-forms` | Every deprecated string extract form (component `text`/`raw_text`/`attr=`/`PATH.prop`/`exists:`, request and message `source`/`field`) loads lowered to its object equivalent and warns `extract-legacy-form` ([SEP-0017](../seps/0017-extract-object.md)) |
| 030 | `extract-object` | The `extract` object on components (every `dom.*` source, a `Name[]` path with `join`, `component.exists`, `pattern` on a component read), requests and messages validates clean ([SEP-0017](../seps/0017-extract-object.md)) |
| 031 | `route-binding-conflict` | A property named like a `:name` binding the view or request still produces → `route-binding-conflict`, whatever its source; a `url.path` entry renames its segment and frees the name ([SEP-0008](../seps/0008-url-properties.md)) |
| 033 | `component-definitions` | File-root `definitions:` are `$ref` targets that are never matched on their own; a view gets one only where it references it, scoped by the reference ([SEP-0019](../seps/0019-component-definitions.md)) |

The `1NN` series verifies the [canonical format](../v1/canonical-format.md) (byte-level formatter output):

| # | Name | Exercises |
|---|---|---|
| 100 | `fmt-quoting` | Quoting preference: plain → single → double |
| 101 | `fmt-key-order` | Fixed key order per entry type |
| 102 | `fmt-list-sort` | Top-level lists alphabetized; nested lists preserve order |
| 103 | `fmt-comment-preservation` | Comments survive rewriting |
| 104 | `fmt-header-preservation` | File header block survives rewriting |
| 105 | `fmt-idempotent` | Formatting is idempotent |
| 106 | `fmt-invalid-untouched` | Invalid files are refused, not rewritten |

## Consumers

- CI schema-validates every `sightmap/*.yaml` here against `sightmap.schema.json` (`npm run validate:conformance`).
- **Most `cases` arrays are not executed yet.** `scripts/validate-sightmap.mjs` reads `expected.json` only to look for `fmt.schema-invalid`/`fmt.parse-error`, which tell it to skip a deliberately-invalid input. It does not run the `command`/`args` or assert the `diagnostics`. The reference implementation's own match runner (`go/match/conformance_test.go`) reads a different corpus, under [`go/conformance/fixtures/`](../../go/conformance/fixtures/).
- **Exception:** the `validate` cases of the request-property, message, message-property, environment, tags, component-privacy, component-watch, extract, URL-property and route-binding fixtures (`017`–`031`) *are* executed. `go/sightmap/spec_conformance_test.go` runs the reference validator (`sightmap.Validate`, the same path the `validate` command uses) over each and asserts its `validate` case's `diagnostics` exactly. Extending this executor to the `match`/`lint`/`explain` commands and the rest of the suite is tracked separately.
- So for every *other* fixture a `cases` entry is still a **contract for ports and a record of intent**, not a passing assertion. Treat a green `validate:conformance` as "these files are schema-valid", nothing more, and keep the Go unit tests as the real coverage for any diagnostic asserted here.
- Community ports in other languages are expected to run the same fixtures via a port-specific runner.
