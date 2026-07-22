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
      "command": "validate" | "match" | "explain" | "lint",
      "args": { "...": "command-specific" },
      "expected": { "...": "subset of the command's --json output to assert" }
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
| 004 | `param-normalization` | Express-style `:param` normalizes to `*` |
| 005 | `selector-array` | `selector` accepts array; alternates tried in order |
| 006 | `view-scoped-vs-global` | Global components match everywhere; scoped only on their view |
| 007 | `request-method-filter` | `match` filters requests by HTTP method |
| 010 | `component-ref` | `$ref` expansion, view attestation, and global+view-scoped dedup ([SEP-0002](../seps/0002-component-ref.md)) |
| 011 | `component-ref-unresolved` | `$ref` to an unknown component → `ref-unresolved` error |
| 012 | `component-ref-circular` | Self-referential `$ref` chain → `ref-circular` error |

## Consumers

- The reference implementation under [`go/`](../../go/) runs these fixtures in CI.
- Community ports in other languages are expected to run the same fixtures via a port-specific runner.
