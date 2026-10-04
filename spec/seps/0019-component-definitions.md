---
sep: 0019
title: Shared component definitions that are not globals
author: Joel Webber (@joelgwebber)
status: Draft
created: 2026-10-04
updated: 2026-10-04
spec-version-target: 1
related-issues: []
related-discussions: []
---

## Summary

Add a file-root `definitions:` list. Its entries are component definitions that are addressable by `$ref` (SEP-0002) exactly like root-level `components:`, but are **never matched on their own**. A view receives a definition only where it references one, scoped by the reference's position. This separates the two jobs a root-level component does today: being *shared* (a `$ref` target) and being *global* (matched on every view).

## Motivation

SEP-0002 builds its `$ref` registry from root-level `components:`, and root-level components are globals: matched against every view. Sharing therefore implies global matching. That is the right default for page chrome (a site header, a footer), and the wrong one for everything else that recurs.

A product card is the common case. It appears in a home-page carousel, in search results and in a recently-viewed rail. Its parts have selectors that are unambiguous only inside the card: `img`, `h3`, `button`, `a.sui-absolute`. Written once and `$ref`'d from each placement, the card's definition becomes a global, and so do its children's selectors. Now `img` claims every image on every page.

This was observed on a generated corpus for a large retail site. The generator moved structures that recurred across views into root-level definitions and `$ref`'d them in place. The result:

- Twenty globals had context-free selectors (`img`, `button`, `label`, `span`, `h1`, `h3`).
- The reference matcher's nearest-enclosing identity hid the damage, because each element still resolved to one name.
- A resolver that lists every matching definition (the overlay extension's `resolveElement`) stacked `FeedbackImage`, `Image` and `SubmitButton` on a single carousel thumbnail.

The only workarounds today are both bad:

- **Inline the definition at every use.** This forfeits SEP-0002's single point of edit and its attestation signal.
- **Pad each shared selector with enough context to be unique page-wide.** This defeats the parent scoping that `children:` exists for, and ties a shared part to one placement's ancestry.

## Proposal

### Shape

```yaml
# Before: sharing forces a global
version: 1
components:
  - name: ProductCard               # global: matched on every view
    selector: '[data-component="ProductCard"]'
    children:
      - name: Title
        selector: h3                # global: every <h3> inside any ProductCard, anywhere

# After
version: 1
definitions:
  - name: ProductCard               # shared: matched only where $ref'd
    selector: '[data-component="ProductCard"]'
    children:
      - name: Title
        selector: h3

views:
  - name: Search
    route: /s/**
    components:
      - name: Results
        selector: ul.results
        children:
          - $ref: ProductCard       # ul.results [data-component="ProductCard"]
```

**JSON Schema diff.** Root object: add `definitions: {type: array, items: {$ref: "#/$defs/componentOrRef"}}` (optional). No `$defs` change.

### Semantics

- **Registry.** The `$ref` registry is built from root-level `components:` **and** `definitions:` across all files. They share one namespace.
- **Matching.** A definition is never part of a page's active component set on its own. `ComponentsForURL` (or its equivalent) returns view-scoped components, which already include `$ref` expansions, plus globals. Definitions contribute only through those expansions.
- **Scoping.** A definition is scoped by the position of the reference, exactly as an inline component would be. A `$ref` at a view's top level is matched within that view. A `$ref` under `children:` is matched within its parent's subtree.
- **Clashes.**
  - A name declared both as a global and as a definition resolves to the global, so adding a definition never changes what an existing `$ref` expands to. The SDK emits `definition-shadowed-by-global` (warning).
  - A definition name declared twice resolves to the first by source-file path, and the SDK emits `merge-collision-definition` (warning).
- **Validation.** Definitions are validated (selectors, properties, unknown fields) even when nothing references them.
- **Lint.** Definitions are linted as scoped components, so rules aimed at broad global selectors do not apply to them.

### Conformance

- MUST include `definitions:` in the `$ref` registry.
- MUST NOT match a definition except through a `$ref` expansion.
- MUST resolve a name declared as both a global and a definition to the global.
- SHOULD emit `definition-shadowed-by-global` and `merge-collision-definition` as above.
- `ref-unresolved` and `ref-circular` apply unchanged.

Fixture `033-component-definitions` covers referenced-versus-unreferenced matching.

## Alternatives considered

- **`global: false` on a root-level component.** This also works, but it overloads one list with two semantics, and a reader has to inspect every entry to know what a file matches. A separate list makes "matched everywhere" versus "matched where referenced" visible at a glance, and it keeps the formatter's per-list sort and the schema simple.
- **Make root-level components non-global, and add an explicit `global: true`.** This is the cleaner end state, but it is a breaking change to every existing corpus, which relies on root-level `components:` matching everywhere.
- **Allow `$ref` to a view-scoped component.** This couples views to one another, and the registry would depend on view order and routing. Shared structure belongs at the root.
- **Do nothing.** The status quo forces the inline-everywhere and pad-every-selector workarounds described in Motivation.

## Migration

Nothing changes for existing corpora: root-level `components:` stay global. Authors and generators can move a root-level component whose selector is only safe in context into `definitions:`. Every view that `$ref`s it keeps working, and views that did not reference it stop matching it.

Tooling:

- The validator and loader learn the new key.
- `fmt` adds `definitions` to the top-level key order and sorts it by `name`.
- Coverage-style tools that treat globals specially should treat definitions as view-scoped.

## Open questions

- Should the overlay wire format (`export`) carry definitions separately? Today views carry each expansion inline and definitions are omitted from the wire, which is sufficient for matching.
- Should a definition that no view references warn (`definition-unused`)?

## References

- [SEP-0002](0002-component-ref.md): component references via `$ref`.
- JSON Schema `$defs`: definitions that are referenced and never validated directly.
