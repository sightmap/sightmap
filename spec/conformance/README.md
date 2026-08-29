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
| 008 | `dependencies-binding` | `dependencies` globs bind definitions to source files ([SEP-0001](../seps/0001-dependencies.md)) |
| 010 | `component-ref` | `$ref` expansion, view attestation, and global+view-scoped dedup ([SEP-0002](../seps/0002-component-ref.md)) |
| 011 | `component-ref-unresolved` | `$ref` to an unknown component → `ref-unresolved` error |
| 012 | `component-ref-circular` | Self-referential `$ref` chain → `ref-circular` error |
| 013 | `route-trailing-slash` | Trailing slashes on the URL path are normalized away before matching |
| 014 | `component-properties` | `properties:` (`name`/`extract`, tree-closed grammar `text`/`attr=`/`PATH.prop`/`exists:PATH`) validates ([SEP-0010](../seps/0010-tree-closed-component-properties.md)) |
| 015 | `view-url` | `url:` on a view (and a file-level default) validates |
| 016 | `stability-tooling-fields` | `stability:` (view + component) validates; reserved tooling fields `access:`/`snapshots:` are permitted |
| 017 | `tags` | `tags:` validates on components (at multiple nesting levels), requests, and views ([SEP-0004](../seps/0004-component-tags.md)) |
| 018 | `request-properties` | `properties:` on a request validates via `field` (body and header paths) and `pattern`; declaring a reserved identity name warns with `request-property-shadows-reserved` ([SEP-0005](../seps/0005-request-properties.md)) |
| 019 | `messages` | `messages:` validates with `level`/`message`/`description`/`source`, including `level: EXCEPTION`; a level-only entry overlapping a level+message entry warns with `message-conflict` ([SEP-0006](../seps/0006-message-entity.md)) |

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
| 108 | `fmt-dependencies-canonical` | `dependencies` lists sorted + deduped |

## Consumers

- CI schema-validates every `sightmap/*.yaml` here against `sightmap.schema.json` (`npm run validate:conformance`).
- **Most `cases` arrays are not executed yet.** `scripts/validate-sightmap.mjs` reads `expected.json` only to look for `fmt.schema-invalid`/`fmt.parse-error`, which tell it to skip a deliberately-invalid input. It does not run the `command`/`args` or assert the `diagnostics`. The reference implementation's own match runner (`go/match/conformance_test.go`) reads a different corpus, under [`go/conformance/fixtures/`](../../go/conformance/fixtures/).
- **Exception:** the message fixtures (`*-messages.fixture`) *are* executed. `go/sightmap/spec_conformance_test.go` runs the reference validator (`sightmap.Validate`, the same path the `validate` command uses) over each and asserts its `validate` case's `diagnostics` exactly. Extending this executor to the `match`/`lint`/`explain` commands and the rest of the suite is tracked separately (one fixture, 017-tags, currently diverges from its own `expected.json` and must be reconciled first).
- So for every *other* fixture a `cases` entry is still a **contract for ports and a record of intent**, not a passing assertion. Treat a green `validate:conformance` as "these files are schema-valid", nothing more, and keep the Go unit tests as the real coverage for any diagnostic asserted here.
- Community ports in other languages are expected to run the same fixtures via a port-specific runner.
