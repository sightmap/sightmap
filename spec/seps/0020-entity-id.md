---
sep: 0020
title: Stable entity identity with `id`
author: Clint Ayres (@jurassix)
status: Accepted
created: 2026-10-09
updated: 2026-10-20
spec-version-target: 1
related-issues: []
related-discussions: []
---

## Summary

Add an `id` to every named entity in a sightmap: environments, views, components, `$ref` entries, properties, requests, payload fields, messages, signals and snapshots. An id is an opaque string, unique across the whole sightmap, that stays the same through renames, moves and edits. It takes no part in matching. A sightmap either has no ids or gives every entity one. With ids, the changes between two versions of a sightmap are a deterministic function of the two files, and concurrent edits merge field by field.

## Motivation

Nothing in a sightmap identifies an entity across versions:

- **Names change and aren't unique.** Authors rename views, components and requests as a product's vocabulary evolves. A component name is also unique only within its parent, so `Title` can appear under several cards.
- **Locators change.** A component's selector changes when its markup does, and because a child is searched inside its parent's match, its effective locator also changes whenever any ancestor's selector does. A view's route and a request's route and method change as URLs are redesigned. An environment's origins change when a deploy target moves to a new host, and its name changes when a team relabels its targets.

Anything that binds to an entity across versions breaks on these changes: data recorded against a component or request, a history of what was deployed to an environment, rules configured per entity, saved queries, review notes, an agent's memory. A consumer that sees `CardDetail` in one version and `PaymentCard` in the next can't know they are the same component.

Comparing two versions doesn't fix this, because two versions don't record intent:

```yaml
# Before                          # After
- name: CardDetail                - name: PaymentCard
  selector: '.card-detail'          selector: '.payment-card'
```

This is either one component renamed with a new selector, or one removed and an unrelated one added. Both histories produce exactly these two files. Heuristics over names, selectors and tree position can guess, but no algorithm can be correct, because the answer isn't in the data.

The same gap shows up when a sightmap is edited concurrently. If one side changes a component's selector while the other moves the component under a new parent, a line-based merge sees one side editing lines the other moved, and conflicts. Resolving it by hand either loses the selector change or duplicates the component. With an id on the component, the two edits are different fields of one entity and combine cleanly.

## Proposal

### Shape

```yaml
# Before
version: 1
views:
  - name: Checkout
    route: /checkout
    components:
      - name: CardDetail
        selector: '.card-detail'
        properties:
          - name: brand
            extract: { from: dom.attr, path: data-brand }
      - $ref: OrderSummary

# After
version: 1
views:
  - id: v8kq2m7rta
    name: Checkout
    route: /checkout
    components:
      - id: c4mxq2nbw7
        name: CardDetail
        selector: '.card-detail'
        properties:
          - id: p9tz2m4kx1
            name: brand
            extract: { from: dom.attr, path: data-brand }
      - id: r3n8kq2mf5
        $ref: OrderSummary
```

**Entities.** Every named object in the schema is an entity and takes an `id`:

| Entity | Where |
|---|---|
| Environment | `environments[]` |
| View | `views[]` |
| Component | `components[]` at file root and in views, `definitions[]`, `children[]` at any depth |
| `$ref` entry | anywhere a component may appear |
| Property | `properties[]` on views, components, requests and messages |
| Request | `requests[]` at file root and in views |
| Payload field | `request.fields[]` and `response.fields[]` on requests |
| Message | `messages[]` |
| Signal | `signals[]` |
| Snapshot | `snapshots[]` |

Value objects (`extract`, `access`, `request`/`response` payloads), `memory` strings, and the `origins` map are not entities. An origin is identified by its map key and is unique by construction.

**JSON Schema diff.**

- `$defs.id`: add `{type: string, pattern: "^[A-Za-z0-9][A-Za-z0-9_-]{7,63}$"}`.
- `$defs.environment`, `view`, `component`, `componentRef`, `componentProperty`, `urlProperty`, `request`, `requestProperty`, `field`, `message`, `messageProperty`, `signal`, `snapshot`: add `id: {$ref: id}` (optional in the schema; see [Adoption](#adoption)).
- `$defs.componentRef`: the entry accepts `id` alongside `$ref` and no other key.

### Semantics

**An id is opaque.** It is 8 to 64 letters, digits, `_` or `-`, starting with a letter or digit, and means nothing beyond identity. Tools that mint ids SHOULD generate them randomly, with at least 8 characters from an alphabet of 32 or more symbols, so ids minted independently, for example on two branches, don't collide. UUIDs fit the pattern.

**Unique across the sightmap.** No two entities may share an id, across all files and all entity kinds. A view and a component can't both be `checkout01`.

**An id names the entity, not its kind of entry.** When an edit changes how an entity is written but not what it is, the id stays. A `$ref` entry replaced by an inline copy of its definition is the same placement and keeps the `$ref` entry's id. The reverse holds too: when an inline component is extracted into a definition or root-level global, the `$ref` entry left in its place is the same placement and keeps the component's id, and the new definition or global is a new declaration with a new id. The component's children move into the new declaration and keep their ids. A component moved from a view to the file root is the same component and keeps its id.

**`$ref` entries are entities.** A `$ref` entry's id names that placement of the definition. A definition's id names the declaration, so it is the same at every placement. Two placements of one definition are told apart by their `$ref` entries' ids. How a consumer combines the two ids for the components inside an expansion is outside this SEP.

**Stable across versions.** A tool that rewrites a sightmap, such as a formatter, an editor or an authoring agent, MUST preserve every id it doesn't deliberately remove, through renames, moves, selector and route changes and any other edit. An id MUST NOT be reused for a different entity after its entity is removed. A tool that mints ids MUST NOT mint one that the sightmap already declares.

**Same entity, same id.** Whether an edit keeps an id is a single question: is the result the same entity? If yes, it keeps the id. If no, it gets a new one.

- When one entity becomes two, one successor keeps the id and the other gets a new id.
- When two become one, one id continues and the other is removed.
- When a copy is made, the original keeps the id and the copy gets a new one.

**An environment's id names the deploy target.** Renaming an environment, or changing its origins, `app_id` or `build_type`, keeps the id: it is still the same target. An environment removed and declared again is the same target only if it keeps its id. A consumer that records anything per environment, such as which version of a sightmap was current there, SHOULD key it by id, so a rename doesn't split the record and a new id doesn't silently continue one.

**What an id is not.** `id` takes no part in route matching, selector matching, which component names an element, specificity, or name resolution. `$ref`, `signals[].ref` and `environments:` lists still refer by name. Two sightmaps that differ only in their ids behave identically.

### Adoption

A sightmap with no ids is valid and behaves exactly as before. Once any entity in a sightmap declares an id, every entity MUST have one. A sightmap is all-or-nothing because a diff or merge keyed by id can only account for entities that carry one: a single id-less entity falls back to the guessing this SEP removes.

A sightmap adopts ids in one pass: a tool mints an id for every entity and writes them back. Adding ids changes no behavior.

### Deriving changes between versions (informative)

With ids, comparing two versions of a sightmap is a total, deterministic function of the two files:

1. Pair entities by `id`.
2. For each pair, compare its fields and its parent's id.
3. An id only in the old version was removed. An id only in the new version was added.

| Change | Paired by id, the result is |
|---|---|
| Rename | Same id, new name |
| A component's own selector changes | Same id, new selector |
| An ancestor's selector changes | Same ids, new effective locator for every component under it |
| Move to a new parent | Same id, new parent id |
| A view's or request's route changes | Same id, new route |
| An environment is renamed | Same id, new name |
| An environment's origins change | Same id, new origins |
| A `$ref` entry replaced by an inline copy | Same id, now a component |
| An inline component extracted into a definition | Same id, now a `$ref` entry; the definition is an addition |
| Delete and re-add with a new id | One removal and one addition. A tool can flag the pair as a likely accidental re-creation when names or selectors match |

Only the endpoints matter: any number of edits between two versions reduce to the same result, and a sequence of versions can be compared pairwise to reconstruct a history. What a consumer does with the result is outside this SEP.

### Merging concurrent edits (informative)

A three-way merge keyed by id (base, ours, theirs) merges field by field. A field changed on one side takes that side's value, and a field changed identically on both sides takes it once. The only conflicts are:

| Conflict | Example |
|---|---|
| Same field, different values | Both sides change one component's selector differently |
| Edit against removal | One side moves a component, the other deletes it |
| Parent cycle | One side moves A under B, the other moves B under A |
| Orphan | One side deletes a parent, the other moves a child into it |
| Id collision | Both sides mint the same new id |

Each is detected mechanically. Everything else, including renames, moves and field edits from both sides on the same entity, merges without conflict. A merge tool of this kind can be registered as a git merge driver for `.sightmap/` files.

### Conformance

A conforming SDK:

- MUST accept `id` on every entity listed in [Shape](#shape), and on a `$ref` entry alongside `$ref`.
- MUST emit `id-invalid` (error) for an id of the wrong shape, or one that isn't a YAML string, such as an unquoted number or a null.
- MUST emit `id-duplicate` (error) when two entities share an id, whatever their kinds or files.
- MUST emit `id-missing` (error) for each entity without an id when any entity in the sightmap declares one.
- MUST NOT let `id` affect matching, naming, specificity or name resolution.

A tool that rewrites sightmaps MUST preserve ids as described in [Semantics](#semantics). The canonical key order puts `id` first in every entity.

## Alternatives considered

**Use `name` as identity.** It's already there, but authors rename freely, and a component name is unique only within its parent. Freezing names to keep identity would trade away the field's actual purpose.

**Derive identity from the name path or the selector path.** Both change whenever an ancestor is renamed or reselected, which is exactly the change that most needs tracking. A parent's selector edit would look like every descendant being replaced.

**Derive a placement's identity from the ids of its ancestors.** This tells a definition's placements apart without an id on the `$ref` entry, but it breaks under common edits. A `$ref` directly under a view has no ancestor with an id, so two views referencing one definition produce the same identity. Wrapping a `$ref` in a new container changes its ancestors and so its identity, though the placement is unchanged. An id on the `$ref` entry has neither problem.

**Ids on components only.** Views, requests, messages and signals have the same rename problem, and a diff or merge that pairs components by id still has to guess for everything else. One rule for every entity is simpler to implement and to explain.

**Infer identity by diffing without ids.** Tree-diff algorithms with similarity scoring produce plausible matchings, but the rename-versus-replace example above has two equally valid answers. Inference can only guess, and silent wrong guesses are worse than an explicit field.

**Keep ids in a separate lockfile.** A sidecar mapping keeps the sightmap uncluttered, but every tool that edits the sightmap would also have to update the sidecar in step. Merge conflicts would move into a second file that people don't read. An inline field travels with its entity.

**Hash each entity.** A content hash changes on any edit, which defeats the point: identity should survive edits.

**Require `id` in the schema.** This makes identity total, but it invalidates every existing sightmap, which needs a new spec stream. All-or-nothing adoption gives every sightmap that opts in the same guarantee without breaking any that haven't.

**Optional ids with a lint warning.** Partial adoption leaves some entities without identity, and every diff and merge has to handle both cases. Requiring ids once any is present keeps the consumer's job simple.

**Mandate UUIDs.** A fixed format guarantees uniqueness by construction, but 36-character ids are noisy in YAML that people read and review. The pattern permits UUIDs and requires 8 characters, which is collision-resistant for sightmap-sized corpora when ids are random.

**Do nothing.** Every consumer that binds to entities keeps reinventing fragile matching on names, selectors and routes, and concurrent editing keeps producing conflicts that lose edits.

## Migration

No existing sightmap changes meaning. A sightmap without ids validates and behaves exactly as before.

- SDKs that reject unknown keys need the schema update to accept the field.
- The reference SDK loads each entity's id, exposes a `$ref` entry's id on the root of its expansion, and implements the three validation errors.
- Tooling that generates or rewrites sightmaps should mint an id for every entity when it adopts ids, and preserve existing ones on every edit.

## Open questions

- **Lineage.** When an entity splits, nothing records that the new successor came from the original. A field for that, such as `formerly: [ids]`, is additive and could follow in its own SEP.
- **References by id.** `$ref`, `signals[].ref` and `environments:` lists refer by name, so a rename still rewrites every reference. Should references be allowed to name an id instead?
- **A diff command.** Should the reference CLI ship the derivation above as a `diff` command, and the id-keyed merge as a merge driver?

## References

- [SEP-0002](0002-component-ref.md): component references, which make one declaration appear at several placements.
- [SEP-0019](0019-component-definitions.md): file-root definitions, the common target of `$ref`.
- [SEP-0009](0009-component-privacy.md) and [SEP-0015](0015-component-watch.md): per-component directives that consumers bind to and need to keep bound across versions.
- React's `key`: a stable identity that makes reconciliation between two trees exact instead of positional.
- Kubernetes `metadata.uid` alongside `metadata.name`: a human name that can change, and an identity that doesn't.
- Terraform `moved` blocks: what a system needs when identity is an address, and every rename has to be declared as a migration.
