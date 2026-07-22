---
sep: 0002
title: Component references via `$ref`
author: Joel Webber (@joelgwebber)
status: Draft
created: 2026-05-19
updated: 2026-05-20
spec-version-target: 1
related-issues: []  # none — design discussion happened in the companion impl PR
related-discussions:
  - "Companion implementation PR (kernel `$ref` expansion + dedup + diagnostics): https://github.com/sightmap/sightmap-js/pull/90"
---

## Summary

Allow an entry in any `components:` array to be a single-key reference object — `{ $ref: ComponentName }` — that expands inline to a deep copy of the named component's full definition. The named component is looked up from a project-wide registry built from each file's root-level `components:` array. This eliminates duplication when the same component (a site header, a persistent footer, a chat widget) appears across many views, and gives views a machine-checkable way to **attest** which globals they expect — a signal drift-detection tooling can act on.

## Motivation

Multi-view sitemaps share common regions across every page. The spec today offers two ways to express that:

- **Repeat the full definition inline in every view** — works, but updating the header's selector or memory entries means editing every view file.
- **Rely on the implicit auto-match of file-root `components:`** — works at match time, but views say nothing about what they expect, so the corpus can't tell "this header was never on this page" from "this header used to be here and is now broken."

Both create real friction. A sightmap for a four-view e-commerce site with shared `SiteHeader`, `NavFooter`, `LiveChatButton`, plus a `Breadcrumb` shared by three of the four, today either duplicates the definitions four ways or accepts a corpus that's silent about which globals each view expects. The first scales badly; the second forfeits drift detection.

A `$ref` entry resolves both. The definition lives in one place, and a view's `components:` array becomes a compact, machine-readable manifest of what that view expects to see.

## Proposal

### Shape

A `components:` array (at file root, within a view, or inside `children:`) may contain entries of two forms:

**Form 1 — inline definition (existing):**

```yaml
- name: SearchBox
  selector: 'form#header-search'
  children:
    - name: SearchInput
      selector: input
```

**Form 2 — reference (new):**

```yaml
- $ref: SearchBox
```

A `$ref` entry MUST contain exactly one key, `$ref`, whose value is the name of a component defined at the root level of some `components:` array in the loaded sightmap. Any other key in the entry is a schema error.

#### Full example

```yaml
# .sightmap/components.yaml — definitions
version: 1
components:
  - name: SiteHeader
    selector: '#header-static'
    description: Outer shell of the site header.
    children:
      - name: SearchBox
        selector: 'form#header-search'
      - name: CartButton
        selector: 'a[data-testid="header-cart-icon"]'

  - name: NavFooter
    selector: '#footer-static'
    children:
      - name: FooterLink
        selector: a
```

```yaml
# .sightmap/views/plp.yaml — view-scoped, attests its expected globals by reference
version: 1
views:
  - name: PLPPage
    route: /b/**
    components:
      - $ref: SiteHeader     # attests SiteHeader is expected on this view
      - $ref: NavFooter      # attests NavFooter is expected on this view
      - name: ProductGrid    # view-specific definition
        selector: '[data-component^="product-results:ResultsWrapped"]'
        children:
          - name: ProductCard
            selector: '[data-testid="product-pod"]'
```

#### JSON Schema diff

The structural change in `spec/v1/sightmap.schema.json`:

**Two new `$defs`:**

- **`componentRef`** — an object with `additionalProperties: false`, `required: ["$ref"]`, and a single property `$ref: { type: "string", minLength: 1 }`.
- **`componentOrRef`** — `oneOf: [ { $ref: "#/$defs/component" }, { $ref: "#/$defs/componentRef" } ]`.

**Three `items:` swaps** — every place a component array exists, its item type widens from a `Component` to a `ComponentOrRef`:

- Top-level `properties.components.items` — `#/$defs/component` → `#/$defs/componentOrRef`.
- `$defs.view.properties.components.items` — `#/$defs/component` → `#/$defs/componentOrRef`.
- `$defs.component.properties.children.items` — `#/$defs/component` → `#/$defs/componentOrRef`.

The `$defs.component` definition itself is unchanged. No existing entry shape becomes invalid; only the `componentRef` shape becomes newly valid where it was previously rejected by `additionalProperties: false` on `component`.

### Semantics

**Expansion.** A `$ref` entry is semantically equivalent to writing the full definition inline at that position. SDKs MUST expand `$ref` before component matching or annotation. The expansion is a **deep copy**; the referencing site owns its expanded copy and subsequent processing (matching, annotation) treats it identically to an inline definition.

**Source attribution.** An expanded clone carries the **registry definition's** `source` (or `sources`, post-SEP-0001) verbatim — not the `$ref` site's. The component is *defined* where its registry entry lives; a `$ref` is an attestation of where it's *expected to appear*, not a redefinition. Tooling that ties code edits to corpus entries via path matching follows the definition, which keeps `$ref` attestations source-free.

**Lookup scope.** The named component is resolved from a **registry** built from the root-level `components:` arrays of all loaded sightmap files. Components nested under `children:`, and components defined inside a view's `components:`, are **not** addressable by `$ref`. Names must be unique within the registry; first-seen wins (sorted by source-file path, as with the rest of the merge). SDKs SHOULD emit a `merge-collision-component` warning on duplicate names.

**Recursive `$ref` in children.** `$ref` is valid in `children:` arrays. When a referenced definition itself contains `$ref` entries in its `children:` (directly or via further indirection), SDKs MUST re-expand those nested references in the deep copy. The cloned subtree is fully resolved before being inlined.

**Circular references.** A `$ref` that, after expansion, would directly or transitively refer to itself is invalid. SDKs MUST detect circular `$ref` chains and MUST emit a validation error with diagnostic code `ref-circular`.

**Unresolved references.** A `$ref` whose value does not name a component in the registry is invalid. SDKs MUST emit a validation error with diagnostic code `ref-unresolved`.

**View attestation.** When a view lists `- $ref: ComponentName`, it is asserting: "this component is expected on this page." SDKs MAY surface "attested but matched 0 elements" as a signal distinct from "this name was never mentioned anywhere." This is a quality-of-implementation feature, not a conformance requirement.

**Deduplication.** When a view attests a component name by `$ref` *and* that name is also present in the file-root registry (i.e. the global auto-match would otherwise add it), the view-attested expansion takes precedence for that view. SDKs MUST NOT emit both the global definition and the view-attested expansion as two separate matches for the same view. The component appears once in the resolved output for that view, scoped to the view.

### Conformance

- SDKs MUST accept `$ref` entries in `components:` arrays at file root, within views, and within `children:`.
- SDKs MUST reject any `$ref` entry that contains keys other than `$ref`.
- SDKs MUST expand `$ref` entries before any component matching, annotation, or lint pass.
- SDKs MUST emit an error-severity diagnostic with code `ref-unresolved` when a `$ref` value does not name a registered component.
- SDKs MUST emit an error-severity diagnostic with code `ref-circular` when a `$ref` chain (after expansion) is self-referential.
- SDKs MUST re-expand any `$ref` entries that appear inside the `children:` of a definition being inlined.
- SDKs MUST carry the registry definition's `source` (or `sources`, post-SEP-0001) into the expanded clone, not the `$ref` site's.
- SDKs MUST NOT produce two distinct matches for the same view when the same name is both globally registered and attested via `$ref` in that view; the view-attested expansion subsumes the global.
- SDKs SHOULD emit `merge-collision-component` warnings on duplicate names in the registry.
- SDKs MAY surface "attested but 0 matches" as a distinct quality signal.

## Alternatives considered

### YAML anchors and aliases (`&foo` / `*foo`)

YAML's native anchor/alias mechanism provides reference semantics within a single document. Ruled out for three reasons:

1. **Cross-document scope.** Anchors do not cross YAML document boundaries. Sightmaps are intentionally split across multiple files; YAML anchors can't reference definitions in sibling files.
2. **Tooling friction.** Many YAML parsers and editors handle anchors poorly. The `eemeli/yaml` and `ruamel.yaml` libraries we target for canonical formatting (`spec/v1/canonical-format.md`) do round-trip anchors, but most editor previews, JSON Schema validators, and ad-hoc YAML scripts do not.
3. **No attestation semantics.** An alias is purely structural; it carries no "this view expects this component" signal that tooling can act on for drift detection.

### Implicit auto-matching only (no mechanism)

Keep the status quo: globals auto-match on every view; views need not mention them. Simple, already supported.

Ruled out because it forfeits the attestation signal. When a global stops matching, there is currently no way for tooling to tell whether the view was ever supposed to have it. Explicit `$ref` in views makes the expectation machine-checkable and gives drift-check tooling a clear hook.

### Separate `uses:` field on views

Add a `uses: [ComponentName, ...]` field alongside `components:`, where `uses` lists expected globals and `components` lists view-specific definitions.

Ruled out: two mechanisms for "components on this view" adds cognitive overhead and creates an ambiguity about where a view-specific component that also references a global should live. A single `components:` array that accepts both inline definitions and `$ref` entries is more uniform and already matches how authors think about the hierarchy.

### Property-level override (`$ref` + override fields)

Allow `$ref` to be combined with additional fields that override specific properties of the referenced definition:

```yaml
- $ref: SiteHeader
  memory:
    - On this page the header collapses on scroll.
```

Deferred rather than ruled out. The current proposal treats `$ref` as pure expansion (no overrides). Override semantics are a natural follow-on once the base mechanism is in place; they can be addressed in a subsequent SEP without breaking anything proposed here. Doing them now would complicate the conformance contract (shallow vs. deep merge? what about `memory`-append vs. `memory`-replace?) for no immediate need.

## Migration

This is an additive, non-breaking change:

- Sightmaps that don't use `$ref` continue to work exactly as before.
- The `$ref` entry shape is new; existing entries are untouched.
- No existing field is renamed, removed, or has its semantics changed.

**SDK impact.** Conforming SDKs MUST add `$ref` support to be conforming under v1 going forward. The kernel work is roughly: schema update to allow the new entry shape, a two-pass loader (build registry, then expand with cycle detection), and a dedup step at match time.

**Non-SDK tooling.** Generic YAML/JSON-Schema tooling that doesn't ship its own validator (editor previews, ad-hoc lint scripts) may see a `$ref` entry before its schema is updated. The schema update in this SEP makes such tools correctly identify the entry once their schema vendor refreshes; until then, they SHOULD treat the entry as unknown-but-tolerable rather than rejecting the file outright. This SHOULD applies to generic tooling, not to conforming SDKs.

## Open questions

1. **Override semantics.** Should a future SEP allow `$ref` + additional fields, producing an inline-override of the referenced definition? If so, what merge strategy applies (shallow merge? deep merge with explicit `memory:` append)?
2. **Cross-sightmap references.** Should `$ref` eventually support referencing components from a different project (a shared component library across multiple sites)? Today's scope is the loaded sightmap only.
3. **`$ref` in `requests:` arrays.** The same duplication problem applies to shared API endpoints. Symmetric application of `$ref` to `requests:` is deferred to a follow-on SEP, but the design here is intentionally compatible with that extension.
4. **"Attested but 0 matches" severity.** Should an attested component with 0 DOM matches be a warning, an error, or implementation-defined? Conditionally-hidden components (e.g. modals not yet opened) make a one-size-fits-all severity hard to set. This SEP leaves it implementation-defined.

## References

- JSON Schema `$ref` — the naming inspiration; our semantics are simpler (name lookup against a registry, no URI resolution, no JSON Pointer paths).
- OpenAPI `$ref` — similar reuse pattern for component schemas.
- [`spec/v1/sightmap.schema.json`](../v1/sightmap.schema.json) — the schema update that lands with this SEP.
- [`spec/v1/schema.md`](../v1/schema.md) — the human-readable counterpart.
- Conformance fixture `010-component-ref.fixture/` — expansion, attestation, and dedup.
- Conformance fixture `011-component-ref-unresolved.fixture/` — `ref-unresolved` error.
- Conformance fixture `012-component-ref-circular.fixture/` — `ref-circular` error.
