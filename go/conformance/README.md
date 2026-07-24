# Conformance fixtures

This directory contains shared test fixtures that any sightmap component-tree
matching implementation must pass. The fixtures are pure data: a component tree,
a sightmap definition, and an expected match output. An implementation is
conformant when it produces the expected output for every fixture.

## Purpose

- Serve as the ground truth for the NFA matching semantics.
- Prevent divergence between Go and any future TypeScript tree-level
  implementation. (No TypeScript test infrastructure is required now — add a
  runner when the TS implementation exists.)
- Document subtle matching edge cases in a format that is both human-readable
  and machine-executable.

## Fixture format

Each fixture lives in its own directory under `fixtures/`:

    fixtures/NNN-description/
      component-tree.json     ComponentNode tree (schema: conformance/component-schema.md)
      sightmap.yaml           One or more component definitions with CSS selectors
      expected-matches.json   { "ComponentName": ["nodeId1", "nodeId2", ...] }
      README.md               What this fixture tests and why

`sightmap.yaml` uses the flat (non-hierarchical) form — each component has a
`name` and a `selectors` list. No `children`, no `$ref`. The full hierarchical
format is tested by the sightmap/ package; these fixtures test the NFA
matching layer only.

`expected-matches.json` maps component name to a sorted list of node IDs.
A node ID that appears under more than one component name would be a test
error (first-match-wins means each node maps to exactly one component).

## Adding a fixture

1. Create `fixtures/NNN-description/` with a descriptive name.
2. Write `component-tree.json` following the schema in `component-schema.md`.
3. Write `sightmap.yaml`:
   ```yaml
   - name: MyComponent
     selectors:
       - "button.primary"
   ```
4. Write `expected-matches.json`:
   ```json
   { "MyComponent": ["node-id-1", "node-id-2"] }
   ```
5. Write `README.md` explaining what the fixture tests.
6. Run `go test ./match/...` to execute the Go conformance runner.

## Running

```
go test ./match/...
```

The Go conformance runner is in `match/conformance_test.go`. It loads every
fixture automatically via `filepath.Glob`.
