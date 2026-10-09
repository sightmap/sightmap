---
sep: 0020
title: Stable component identity with `id`
author: Clint Ayres (@jurassix)
status: Draft
created: 2026-10-09
updated: 2026-10-09
spec-version-target: 1
related-issues: []
related-discussions: []
---

## Summary

Add an optional `id` to components: an opaque string that is unique among the sightmap's component declarations and stays the same through renames, moves and selector changes. It takes no part in matching. It lets any tool tell, from two versions of a sightmap, exactly which component became which, and lets two people or tools edit one sightmap concurrently without losing track of what they each changed.

## Motivation

A component's `name` and `selector` are both poor identities:

- **Names change and aren't unique.** Authors rename components as a product's vocabulary evolves. A name is also unique only within its parent, so `Title` can legitimately appear under several cards.
- **Selectors change for two reasons.** A component's own selector changes when its markup does. Because a child's selector is searched inside its parent's match, a component's effective locator, the path of selectors from the root, also changes whenever **any ancestor's** selector changes. One edit to a container changes the locator of everything inside it.

Anything that binds to a component across versions breaks on these changes: data recorded against a component, rules configured per component, saved queries, review notes, an agent's memory. A consumer that sees `CardDetail` in one version and `PaymentCard` in the next has no way to know they are the same component.

Comparing two versions without ids doesn't fix this, because two versions don't record intent:

```yaml
# Before                          # After
- name: CardDetail                - name: PaymentCard
  selector: '.card-detail'          selector: '.payment-card'
```

This is either one component renamed with a new selector, or one component removed and an unrelated one added. Both histories produce exactly these two files. Heuristics over names, selectors and tree position can guess, and usually guess right, but no algorithm can be correct, because the answer isn't in the data.

The same gap shows up when a sightmap is edited concurrently. If a tool proposes a selector change for a component while a person moves that component under a new parent, a line-based merge sees one side editing lines the other side moved, and conflicts. Resolving it by hand either loses the selector change or duplicates the component. If both sides keep an id on the component, the edits are two different fields of one entity and combine cleanly.

## Proposal

### Shape

```yaml
# Before
version: 1
components:
  - name: CheckoutForm
    selector: form.checkout
    children:
      - name: CardDetail
        selector: '.card-detail'

# After
version: 1
components:
  - id: f3n8kq2m
    name: CheckoutForm
    selector: form.checkout
    children:
      - id: kq2m7rta
        name: CardDetail
        selector: '.card-detail'
```

**JSON Schema diff.**

- `$defs.componentId`: add `{type: string, pattern: "^[A-Za-z0-9][A-Za-z0-9_-]{3,63}$"}`.
- `$defs.component`: add `id: {$ref: componentId}` (optional).
- `$defs.componentRef`: unchanged. A `$ref` entry still accepts no other key, so it carries no `id`.

### Semantics

**An id is opaque.** It is 4 to 64 letters, digits, `_` or `-`, starting with a letter or digit, and means nothing beyond identity. Tools that mint ids SHOULD use at least 8 random characters from an alphabet of 32 or more symbols, so ids minted independently, for example on two branches, don't collide.

**Unique among declarations.** No two component declarations may share an id, across all files, globals, definitions, view components and nesting levels. Uniqueness is a property of declarations, not of the expanded component tree: a definition referenced by `$ref` from several places is one declaration, so its id appears at each placement.

**Placements have id paths.** A placement's identity is its id path: its own id, preceded, inside a `$ref` expansion, by the id path of the component that holds the `$ref`. Outside any expansion the id path is just the component's id, so a move, or a rename or selector change on an ancestor, never changes it. Only the reference sites a placement is reached through contribute to its id path.

```yaml
definitions:
  - id: kq2m7rta
    name: ProductCard
    selector: '[data-component="ProductCard"]'

views:
  - name: Search
    route: /s/**
    components:
      - id: r2v8yd5e
        name: Results
        selector: ul.results
        children:
          - $ref: ProductCard     # id path [r2v8yd5e, kq2m7rta]
      - id: f4j8nz1q
        name: Recent
        selector: ul.recent
        children:
          - $ref: ProductCard     # id path [f4j8nz1q, kq2m7rta]
```

Moving `Results` keeps the first placement's id path. Moving the `$ref` itself to another holder makes a different placement, with a new id path. An id path is complete when every component holding an enclosing `$ref` declares an id; a consumer that needs per-placement identity should treat an incomplete path as unidentified. Children of a placed definition extend the same prefix: `Title` inside the first `ProductCard` has the id path `[r2v8yd5e, <title-id>]`.

**Stable across versions.** A tool that rewrites a sightmap, such as a formatter, an editor or an authoring agent, MUST preserve every id it doesn't deliberately remove, through renames, moves, selector changes and any other edit. An id MUST NOT be reused for a different component after its component is removed. A tool that mints ids MUST NOT mint one that the sightmap already declares.

**Splits and merges.** When one component becomes two, one successor keeps the id and continues its identity, and the other is a new component with a new id. When two become one, one id continues and the other is removed.

**What an id is not.** `id` takes no part in route matching, selector matching, which component names an element, or specificity. Two sightmaps that differ only in their ids match every page identically.

**Partial adoption.** A sightmap may give ids to some components and not others. Identity is tracked only for components that carry one, which is why lint flags the rest once any id appears.

### Deriving changes between versions (informative)

With ids, comparing two versions of a sightmap is a total, deterministic function of the two files, with no heuristics:

1. Pair declarations by `id`.
2. For each pair, compare `name`, parent id, and locator (the path of selectors from the root).
3. An id only in the old version was removed; an id only in the new version was added.

| Change | Paired by id, the result is |
|---|---|
| Rename | Same id, new name, same locator |
| A component's own selector changes | Same id, new locator for it |
| An ancestor's selector changes | Same ids, new locator for every component under that ancestor |
| Move to a new parent | Same id, new parent id, new locator for it and its descendants |
| Delete and re-add with a new id | One removal and one addition; a tool can flag the pair as a likely accidental re-creation when names or selectors match |
| Split | The successor that keeps the id continues; the other successor is an addition |

Only the endpoints matter: any number of edits between two versions, at any depth, reduce to the same result, and a sequence of versions can be compared pairwise to reconstruct a full history. What a consumer does with the result is outside this SEP.

### Merging concurrent edits (informative)

A three-way merge keyed by id (base, ours, theirs) merges field by field: a field changed on one side takes that side's value, and a field changed identically on both sides takes it once. The only conflicts are:

| Conflict | Example |
|---|---|
| Same field, different values | Both sides change one component's selector differently |
| Edit against removal | One side moves a component, the other deletes it |
| Parent cycle | One side moves A under B, the other moves B under A |
| Orphan | One side deletes a parent, the other moves a component into it |
| Id collision | Both sides mint the same new id |

Each is detected mechanically. Everything else, including renames, moves and selector edits from both sides on the same component, merges without conflict. A merge tool of this kind can be registered as a git merge driver for `.sightmap/` files.

### Conformance

A conforming SDK:

- MUST accept `id` on components, and MUST accept and ignore it if it doesn't track identity.
- MUST emit `component-id-invalid` (error) for an id of the wrong shape.
- MUST emit `component-id-duplicate` (error) when two component declarations share an id. A definition referenced from several places MUST NOT be reported as a duplicate.
- SHOULD emit `component-id-missing` (warning) from lint for each component declaration without an id, when the sightmap declares any component id.
- MUST NOT let `id` affect matching, naming or specificity.

A tool that rewrites sightmaps MUST preserve ids as described in [Semantics](#semantics). The canonical key order for a component puts `id` first.

## Alternatives considered

**Use `name` as identity.** It's already there, but authors rename components freely, and a name is unique only within its parent, so it can't identify a component on its own. Freezing names to keep identity would trade away the field's actual purpose.

**Derive identity from the name path or the selector path.** Both change whenever an ancestor is renamed or reselected, which is exactly the change that most needs tracking. A parent's selector edit would look like every descendant being replaced.

**Infer identity by diffing without ids.** Tree-diff algorithms with similarity scoring produce plausible matchings, but the rename-versus-replace example above has two equally valid answers. Inference can only ever be a guess, and silent wrong guesses are worse than an explicit field.

**Keep ids in a separate lockfile.** A sidecar mapping keeps the sightmap uncluttered, but every tool that edits the sightmap would also have to update the sidecar in step, and every rename or move would need a matching sidecar edit. Merge conflicts would move into a second file that people don't read. An inline field travels with the component.

**Hash each definition.** A content hash changes on any edit, which defeats the point: identity should survive edits.

**Require `id`.** Required ids would make identity tracking total, but would invalidate every existing sightmap. An optional field with a lint warning once a sightmap adopts ids gets the same result for sightmaps that opt in, without breaking anyone.

**Mandate UUIDs.** A fixed format would guarantee uniqueness by construction, but 36-character ids are noisy in YAML that people read and review. The pattern permits UUIDs and recommends at least 8 random characters, which is collision-resistant for sightmap-sized corpora.

**Do nothing.** Every consumer that binds to components keeps reinventing fragile matching on names and selectors, and concurrent editing keeps producing conflicts that lose edits.

## Migration

No existing sightmap changes meaning. `id` is optional, and a sightmap without it validates and matches exactly as before.

- SDKs that reject unknown keys need the schema update to accept the field.
- The reference SDK loads it, exposes each component's id and id path, and implements the two validation errors and the lint warning.
- Tooling that generates or rewrites sightmaps should start minting ids for new components and preserving existing ones. Adding ids to an existing sightmap is a one-time pass with no effect on matching.

## Open questions

- **Lineage.** When a component splits, nothing records that the new successor came from the original. A field for that, such as `formerly: [ids]`, is additive and could follow in its own SEP if a consumer needs history to carry across a split; nothing in this SEP depends on it.
- **Other entities.** Views, requests, messages and signals have the same rename problem. Should they get `id` in this SEP or a follow-up?
- **Placement ids.** Id paths identify a definition's placements without new syntax. Should a `$ref` entry instead be allowed its own id, at the cost of relaxing "a reference carries no other key"?
- **A diff command.** Should the reference CLI ship the derivation above as a `diff` command, and the id-keyed merge as a merge driver?
- **Minimum length.** Four characters keeps hand-written ids possible; eight would make collisions between independently minted ids negligible. Which floor belongs in the schema rather than in guidance?

## References

- [SEP-0002](0002-component-ref.md): component references, which make one declaration appear at several placements.
- [SEP-0019](0019-component-definitions.md): file-root definitions, the common case for id paths.
- [SEP-0009](0009-component-privacy.md) and [SEP-0015](0015-component-watch.md): per-component directives that consumers bind to and need to keep bound across versions.
- React's `key`: a stable identity that makes reconciliation between two trees exact instead of positional.
- Kubernetes `metadata.uid` alongside `metadata.name`: a human name that can change, and an identity that doesn't.
- Terraform `moved` blocks: what a system needs when identity is an address, and every rename has to be declared as a migration.
