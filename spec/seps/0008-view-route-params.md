---
sep: 0008
title: Parameterized view routes and view `properties[]`
author: Clint Ayres (@jurassix)
status: Draft
created: 2026-08-05
updated: 2026-08-05
spec-version-target: 1
related-issues: []
related-discussions: []
---

## Summary

Give a `:name` segment in a `View`'s `route` a binding, not just a match: `route: /shop/p/:product_id` matches `/shop/p/12345` exactly as `/shop/p/*` does today, but additionally binds `product_id = "12345"` as a view property. Add an optional `properties: ViewProperty[]` array, mirroring SEP-0003's component `properties[]` and SEP-0005's request `properties[]`, so a view can also name a query-string value or override/transform a bound path param. This is the third instance of the same mechanism (component, request, now view) and the first that resolves from a URL string alone, with no live DOM or live traffic required.

## Motivation

A `View`'s `route` can already glob-match a dynamic URL segment (`/shop/p/*`), but nothing captures *what value* matched. An agent looking at a product page today gets the view identity `ProductDetail` and nothing else: the `12345` that made it that particular product page is thrown away at match time. Four concrete cases:

- **Product detail.** `/shop/p/12345` resolves to a page named `ProductDetail`; nothing lets a consumer read back which product.
- **Multi-tenant path segment.** `/org/:org_id/settings`: the org ID is the single most useful discriminator on this view, and it lives in the path today with no way to extract it.
- **A/B variant via query string.** `/shop/p/12345?variant=blue`: `variant` never reaches a consumer at all, since route matching ignores the query string entirely ([`schema.md`](../v1/schema.md#route-matching): "Query string and fragment are ignored").
- **Classifying by a path value.** A `signals:` rule ([SEP-0007](0007-signals.md)) that should fire only for a specific org or product has no property on a `View` ref to filter against; SEP-0007's own conformance section currently states a view "has no extractable property at all."

Today, `:param` segments are defined only for `Request` routes, where they normalize to `*` ([`schema.md`](../v1/schema.md#route-matching)). Nothing stops an author from writing `:product_id` on a *view* route today (the view specificity table already scores `:param` as `2`), but the segment binds nothing. The score exists; the binding does not.

## Proposal

### Shape

A `:name` segment in a view `route` matches exactly one path segment, identically to `*`, and additionally binds that segment's percent-decoded value as a view property named `name`.

```yaml
# Before: the product ID is unrecoverable
- name: ProductDetail
  route: /shop/p/*

# After, common case: nothing but the route
- name: ProductDetail
  route: /shop/p/:product_id

# After, escape hatch: query param, rename, transform
- name: ProductDetail
  route: /shop/p/:product_id
  properties:
    - name: variant
      extract: query:variant
    - name: product_id
      extract: param:product_id
      transform: number
```

`ViewProperty` mirrors `componentProperty`'s field-for-field shape (SEP-0003):

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | yes | Key used in the property annotation. Must match `^[a-z][a-z0-9_]*$`, matching `componentProperty.name`'s pattern. |
| `extract` | string | yes | Extraction directive: `param:<segment_name>` or `query:<key>`. See [Extraction directives](#extraction-directives). |
| `transform` | string | no | Same enum as `componentProperty.transform`: `first_word`, `last_word`, `first_number`, `first_dollar`, `number`, `slug`. |

`extract` is required even for a path param already named by the route. The redeclaration is only needed when an author wants a `transform` or wants the property under a different name than the segment; requiring it uniformly avoids a separate defaulting rule for the plain-binding case (see [Alternatives considered](#alternatives-considered) for the alternative of making it optional).

**Collision rule.** An explicit `properties:` entry whose `name` matches an implicit route binding overrides that binding, the same shape as SEP-0003's reserved `value` name overriding the AX built-in. In the example above, `product_id` is both bound by the route and redeclared in `properties:` to attach `transform: number`; the redeclaration wins.

### Extraction directives

`extract` accepts exactly two forms:

- **`param:<segment_name>`**: the value bound by a `:segment_name` in this view's own `route`. Must resolve to a `:param` actually present in the route.
- **`query:<key>`**: the first percent-decoded value of the named query-string parameter, case-sensitive key match. Absent when the key isn't present on the matched URL.

**The inclusion test.** A URL part belongs in this grammar when a *named slice* of it is the whole answer a consumer needs. `param:` and `query:` both pass this test: extraction alone fully resolves them, and neither one touches route matching. Two other candidates fail it, for two different reasons, and are handled differently below:

- **`host` and `fragment` are deferred, not ruled out.** Both are whole strings an author will plausibly want to *parameterize*, not merely extract as an opaque value: a subdomain-per-tenant deployment (`acme.app.com/settings` vs. `globex.app.com/settings`) wants `:tenant.app.com` in the route itself, and a hash-routed SPA (`#/orders/123`) wants `:param` matching *inside* the fragment, since route matching ignores the fragment today. Shipping a bare `extract: host` or `extract: fragment` now answers only the shallow version of both cases and risks foreclosing the better one. Left as an [open question](#open-questions).
- **`path` and `full` are ruled out**, not deferred; see [Alternatives considered](#alternatives-considered). Both hand back a whole-URL string, which is the exact high-cardinality value this SEP exists to let an author collapse into named parts.

### Semantics

`:name` matching is layered on the existing glob matcher, not a new mechanism: `/shop/p/:product_id` matches a URL identically to `/shop/p/*` (same segment, same specificity score of `2`), and additionally records the segment's percent-decoded text under the name `product_id`. `**` cannot bind, since it spans a variable number of segments and there is no single value to name.

`query:` extraction operates on the matched URL's query string independent of route matching; matching itself continues to ignore the query string and fragment entirely, unchanged from today.

Because both `param:` and `query:` values are read directly off the URL string, not off live DOM state (SEP-0003) or live request/response traffic (SEP-0005), they resolve in a **static context** too: a saved session, a coverage report, or a lint pass over a URL string alone can produce every declared view property with no live observation required. This is the one respect in which view `properties:` differs from its two precedents.

### Conformance

- MUST match a `:name` segment as exactly one path segment, with the same matching behavior as `*` at that position.
- MUST bind `:name`'s segment value, percent-decoded, as a view property named `name`, wherever the view resolves as the current view.
- MUST NOT bind a value for `*` or `**`.
- View specificity scoring is unchanged: `:param` continues to score `2`, exactly as today's table already states.
- MUST reject a `:name` segment whose name doesn't match `^[a-z][a-z0-9_]*$` (diagnostic: `route-param-invalid`).
- MUST reject a route declaring the same `:name` more than once (diagnostic: `route-param-duplicate`).
- MUST reject a `properties:` entry with `extract: param:<n>` where `<n>` is not a `:param` present in that view's own `route` (diagnostic: `view-property-param-unresolved`).
- MUST take the first occurrence when a query key repeats in the URL; key comparison is case-sensitive.
- MUST omit a `query:` property silently when its key is absent from the matched URL: no diagnostic, mirroring SEP-0003/SEP-0005's silent-omission rule.
- MUST NOT let a `query:` declaration change route-matching behavior; the query string remains unmatched.
- MUST resolve every declared view property (`param:` and `query:` alike) in a static context: a URL string with no live DOM or live traffic available. This is a stricter requirement than SEP-0003/SEP-0005 place on their own properties, which MUST be omitted (not resolved) offline; an SDK implementing this SEP MUST NOT apply that same offline-omission rule to view properties.
- SHOULD apply `transform` identically to how SEP-0003 applies it (skip on empty/absent raw value; single transform only, not composable).

### JSON Schema diff

`$defs.view.properties` gains one new optional property:

```
$defs.view.properties.properties:
  type: array
  items: { $ref: "#/$defs/viewProperty" }
  description: "Named values bound from this view's own route params and/or the matched URL's query string."
```

`$defs.view.properties.route.description` is amended to document `:name` binding, alongside its existing glob-matching description.

New `$defs` entry:

```
$defs.viewProperty:
  type: object
  required: [name, extract]
  additionalProperties: false
  properties:
    name:
      type: string
      pattern: "^[a-z][a-z0-9_]*$"
      description: "Key a consumer refers to this value by."
    extract:
      type: string
      pattern: "^(param:[a-z][a-z0-9_]*|query:.+)$"
      description: "param:<name> reads a :name route-segment binding; query:<key> reads the matched URL's query string."
    transform:
      type: string
      enum: [first_word, last_word, first_number, first_dollar, number, slug]
      description: "Optional post-processing applied to the extracted string. Same vocabulary as componentProperty.transform (SEP-0003) and requestProperty.transform (SEP-0005)."
```

`$defs.signal.properties.ref.description` (SEP-0007) would additionally need amending if the SEP-0007 relationship below is adopted. Noted here for visibility, but that edit is out of scope for this PR (see [Migration](#migration)).

### SEP-0007 relationship

SEP-0007's conformance section currently states an SDK "MUST reject any `filter:` key on a `Message` or `View` ref... a view has no extractable property," and its open question #2 asks whether views are worth referencing at all, calling them "the least load-bearing of the four entity kinds." Once a view has declared properties, that's no longer true:

```yaml
signals:
  - name: product.detail.viewed
    ref: ProductDetail
    filter:
      product_id: "12345"
```

This SEP does not itself change SEP-0007's text (see [Migration](#migration) for why), but the maintainers accepting this SEP should treat SEP-0007's reject rule as needing to narrow to `Message` refs only, once both land.

## Alternatives considered

### 1. Explicit `properties:` only, no implicit route binding

Require every bound value, including plain path params, to be declared via `properties: [{name: product_id, extract: param:product_id}]`, fully mirroring SEP-0003 and SEP-0005, where nothing is ever extracted without an explicit declaration.

Ruled out: the route already names the param (`:product_id`); forcing a second declaration of the same name to bind the same value makes the common case (a plain path param, no transform, no rename) strictly more verbose for no added clarity.

### 2. Implicit binding only, no `properties:` array on views at all

Ship `:name` binding with nothing else: no query extraction, no transform, no rename.

Ruled out: the query-string gap is already a known, named gap (see Motivation), and would immediately need a follow-up SEP. Shipping the binding without the array either leaves that gap open or forces a second SEP one release later for a mechanism this one could carry now.

### 3. `extract: path` / `extract: full` (whole-URL extraction)

Add a mode that extracts the full matched path, or the full URL, as a single property value.

Ruled out on purpose, not deferred: both hand back exactly the high-cardinality whole-string value this SEP exists to let an author collapse into named, low-cardinality parts. A corpus reaching for `extract: full` has skipped the work of naming its params, which is the point of the SEP.

### 4. Do nothing: leave path/query values to bespoke, off-spec extraction

The status quo: any consumer wanting a value out of a URL builds its own ad hoc regex or string-splitting logic, independently, with no shared vocabulary and no way for an agent to discover what's available without reading that consumer's code.

Ruled out per Motivation.

## Migration

Purely additive for existing corpora. `route: /shop/p/:product_id` already validates against today's schema and already scores `2` in view specificity, so nothing that validates today becomes invalid, and no existing match outcome changes. The one behavior change worth naming explicitly: a view route already using `:param` today gains a property value it did not previously produce. This is new output, not a changed match.

[`spec/conformance/004-param-normalization.fixture`](../conformance/004-param-normalization.fixture) does not regress: its `:param` (`route: /api/users/:id`) is on a *request*, and its view uses `*` (`route: /api/users/*`), so a views-only change leaves both of that fixture's expected results untouched.

Under `additionalProperties: false`, an existing SDK encountering a `properties:` key under a `view:` entry rejects the corpus, the same constraint SEP-0003 and SEP-0005 already documented for their own `properties:` additions. Release playbook, matching that precedent:

1. This SEP merges (status: Accepted).
2. `sightmap/sightmap` implements view-route param binding and `properties:` resolution. Bumped to a minor release.
3. If maintainers agree the SEP-0007 amendment described above should land, it ships as a small follow-up PR to `0007-signals.md`'s own text, not as part of this PR: SEP-0007 is itself still an open Draft, and editing it here would create merge conflict with its own in-flight PR.
4. Consumers (e.g. Subtext) pin `github.com/sightmap/sightmap/go >= <new version>` before authoring `:name` bindings or `properties:` on a `views:` entry.

This SEP also reserves conformance fixture number `021` by convention (the next unused `0NN` slot after `020-signals.fixture`); the fixture itself ships with the implementation PR, per this repo's established pattern of doc-PR-then-impl-PR for SEP-0005/0006/0007.

## Open questions

1. **`host` and `fragment` extraction.** Both are deferred, not ruled out, for a design reason rather than a scope reason: both look like they eventually want `:param`-style parameterization (`:tenant.app.com`; `:param` matching inside a hash-routed fragment) rather than a bare opaque-string read, and shipping the shallow read now risks needing to support both forms side by side later. Is the hash-routed-SPA case, where the entire route lives in the fragment and is unmappable by this spec today, common enough that reviewers want the shallow `extract: fragment` read now, ahead of a real parameterization design?
2. **Extending `:param` binding to `Request` routes.** SEP-0005's `field` grammar addresses `req`/`rsp` body and headers, but not the request's own URL params. A `req.url.param.<name>` addition to that grammar is a natural follow-on but is scoped out of this SEP to keep it to one decision.
3. **PII and cardinality.** A bound path segment or query value can be an email address, a token, or other sensitive data. This SEP takes the same position SEP-0005 already took on redaction: sightmap declares where the value lives, and whichever consumer resolves it against real traffic owns redaction and any placeholder convention that implies. Does the spec need even an advisory marker for this, or is silence (as with SEP-0005) sufficient?
4. Should a corpus-linting tool check a view's `url:` (its representative concrete URL) against its own `route:`'s `:param` segments, now that those segments are semantically meaningful rather than purely cosmetic?

## References

- [SEP-0003](0003-component-properties.md): the DOM `properties[]` precedent this SEP's `ViewProperty` shape mirrors field-for-field.
- [SEP-0005](0005-request-properties.md): the request-side `properties[]` precedent; shares this SEP's `name`/`transform` vocabulary.
- [SEP-0004](0004-component-tags.md): the same "generalize a component-only mechanism once a second entity needs it" shape this SEP repeats for a third entity.
- [SEP-0007](0007-signals.md): `filter:` on a `ref:`, and the open question this SEP partially answers about whether a `View` ref is worth having.
- Prior art for `:param` route syntax: Express.js route parameters; the WHATWG [`URLPattern`](https://urlpattern.spec.whatwg.org/) web API's named group syntax.
