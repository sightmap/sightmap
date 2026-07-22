---
sep: 0001
title: Additive `dependencies[]` field on view + component entries
author: Chip Lay (@chiplay)
status: Draft
created: 2026-05-19
updated: 2026-05-20
spec-version-target: 1
related-issues: []
related-discussions: []
---

## Summary

This SEP adds an optional `dependencies: string[]` property to view and component entries. The strings are minimatch globs with `!`-prefix negation, anchored at the project root (the directory containing `.sightmap/`). They name supplementary files whose changes SHOULD trigger re-curation of the entry. The existing `source` field is unchanged and remains the canonical primary binding. The change is purely additive: it ships under the existing spec stream (`version: 1`) with no migration tooling.

## Motivation

- `source` is the only declarative path binding today. Reverse-lookup (`match --path <file>`) only finds direct hits, so any file that influences a view but is not its primary source is invisible to the lookup.
- Curators implicitly know that a view "also depends on" certain hooks, stores, and styles. That knowledge is not captured anywhere a tool can read.
- Consumers — the sightmap-assistant subagent, IDE integrations, bundlers — cannot make change-propagation decisions without that knowledge.
- The only workaround in the current spec is to force every supplementary file into its own component entry. This produces entries that are not semantically component-shaped (hooks, services, utils, stylesheets) and pollutes the corpus.

## Proposal

### Shape

Both view and component subschemas gain an optional `dependencies` property and keep `additionalProperties: false`:

```json
"dependencies": {
  "type": "array",
  "items": { "type": "string" },
  "description": "Optional list of supplementary files (minimatch globs, '!' negation, project-root-anchored) whose changes should trigger re-curation of this entry. See Semantics for scope rules."
}
```

### Semantics

The falsifiable rule for what belongs in `dependencies[]`:

> Files whose changes SHOULD trigger re-curation of this entry.

What belongs:

- Hooks the entry consumes (e.g. `useChecklist`)
- Services, stores, and shared utils
- CSS and style files the entry loads
- Helper modules that do not warrant their own entry

What does NOT belong:

- Tests
- Type-only imports
- Framework code (React, Vue, etc.)
- Any file that already has its own component or request entry — use the existing entry-level binding, do not restate it

Strings in `dependencies[]` are interpreted as minimatch globs, project-root-anchored at the directory containing `.sightmap/`. A `!` prefix negates. When multiple positive globs match the same file, the first positive glob in declaration order wins for provenance reporting (`matchedBy` in `match --path` output).

### Normative rules

1. An entry's resolved `dependencies[]` set MUST NOT contain its own `source`. Diagnostic code: `dependencies.self-redundant`.
2. An entry's resolved `dependencies[]` set MUST NOT contain a path that is the `source` of any other entry in the same `.sightmap/`. Diagnostic code: `dependencies.overlaps-entry`.
3. A glob in `dependencies[]` that resolves to zero files MUST surface a diagnostic. The existing `unknown-source` diagnostic vocabulary is narrowed to apply to `dependencies[]` globs.

### Conformance

A conforming SDK MUST:

- Accept `dependencies: string[]` as an optional property on view and component entries, at every depth (including recursive `children`)
- Interpret strings as minimatch globs, project-root-anchored, with `!`-prefix negation
- Surface diagnostic `dependencies.self-redundant` when an entry's resolved `dependencies[]` contains its own `source`
- Surface diagnostic `dependencies.overlaps-entry` when an entry's resolved `dependencies[]` contains a path that is another entry's `source`
- Surface the zero-match diagnostic (`unknown-source` vocabulary, narrowed) when a glob in `dependencies[]` resolves to no files
- Canonicalize `dependencies[]` arrays by lexicographic sort + deduplication (per `spec/v1/canonical-format.md`)
- Pass the shared conformance fixtures `conformance/008-dependencies-binding.fixture/` and `conformance/108-fmt-dependencies-canonical.fixture/`

A conforming SDK MAY choose its glob library if it passes the shared fixtures.

## Compatibility & Coordinated Release

### The additive-field paradox

`additionalProperties: false` means a document using a newly-added optional field is schema-invalid under older SDKs. Adding a field has the same compatibility consequence as removing one, just in the opposite direction. Concretely: an author who adds `dependencies: [...]` to `.sightmap/app.yaml` sees `@sightmap/sightmap@0.10.x` and earlier reject the config with `must NOT have additional properties`.

### Coordinated release matrix

| Component | Action | Version |
|---|---|---|
| `sightmap/spec` | This PR merges; new schema published | n/a (spec stream stays at `1`) |
| `@sightmap/sightmap` | Re-vendor schema; ship `match --path` + new `check` diagnostics | `0.11.0` |
| `@sightmap/mcp` | Lockstep changeset bump | `0.11.0` |
| All other monorepo packages | Lockstep via changesets | `0.11.0` |
| `@sightmap/plugin` | Bump host min-version to `@sightmap/sightmap@>=0.11.0`; surface new diagnostics in hook output | Separate release, after sightmap-js `0.11.0` publishes |

The detailed playbook (exact changesets, version-pin examples, host migration steps) lives in `sightmap-js/docs/releases/0.11.0-coordinated.md`, drafted alongside the sightmap-js step-2 PR.

### Adopter guidance

- Adopters MUST pin to `@sightmap/sightmap@>=0.11.0` before adding `dependencies` to any `.sightmap/` corpus.
- Hosts (plugin consumers) MUST require `>=0.11.0` of the sightmap CLI.
- This requirement is permanent: every future additive field carries the same constraint (see `spec/VERSIONING.md`).

## Alternatives considered

1. **Replace `source` with `sources: string[]`** (the original SEP-0001 direction). Rejected after team feedback: forces every entry through an array shape; collapses the "what this entry IS" vs "what this entry ALSO depends on" distinction; requires migration tooling; basis of the closed [sightmap/spec#23](https://github.com/sightmap/spec/pull/23).
2. **Name the field `references`.** Rejected to avoid collision with SEP-0002's incoming `$ref` field.
3. **Apply to `requests` in this round.** Deferred. The use case for "additional files this request depends on" is less developed; it can be added later via additive SEP if needed.
4. **Make `dependencies` runtime-affecting** (e.g. pre-load referenced files into the browser-driving adapter's cache). Rejected: runtime adapters read DOM/fiber state, not source files; the runtime cost increase is not warranted by the curation-time use case.

## Migration

No corpus migration is needed — the field is additive and optional. Existing `.sightmap/` directories remain valid without modification.

**Adopter pin requirement.** Hosts and adopters MUST pin to `@sightmap/sightmap@>=0.11.0` before adopting the field; older SDKs reject configs using `dependencies` as schema-invalid (per the `additionalProperties: false` policy in `spec/VERSIONING.md`).

**Adoption plan (rollout DAG).**

1. This SEP merges into `spec/main`.
2. Coordinated `0.11.0` cut across the `sightmap-js` monorepo (re-vendor schema; ship `match --path` extension; ship new `check` diagnostics; publish all packages via changesets).
3. `@sightmap/plugin` update — host min-version bump; surface new diagnostics. Separate PR, gated on (2).
4. `sightmap-python` catch-up — schema re-vendor, parse `dependencies`. Independent of (3); can run in parallel after (2).
5. Docs catch-up — `sightmap.org` authoring guide updates. Independent of (3) and (4).

Steps 2–5 are gated on (1). Step 3 is gated on (2). Steps 4 and 5 are independent of each other.

## Open questions

None as of 2026-05-20 — reserved for review feedback.

## References

- Closed [sightmap/spec#23](https://github.com/sightmap/spec/pull/23) — rejected `source → sources[]` direction
- Companion sightmap-js step-2 design (pending): `match --path` extension, `check` diagnostics, SPEC-PIN re-vendor
- Empirical basis: Sightmap Pit-of-Success Audit — Fullstory Web (May 2026)
