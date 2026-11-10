---
sep: 0025
title: Query conditions in routes
author: Joel Webber (@joelgwebber)
status: Draft
created: 2026-11-01
updated: 2026-11-01
spec-version-target: 1
related-issues: []
related-discussions: []
---

## Summary

A view or request `route` may end in **query conditions**: `/profile?section=gift-cards`, `/page?id=:id`, `/graphql?operationName=Items`. Each condition names a query key and reads that key's first decoded value: `KEY=VALUE` requires that value, `KEY=:name` and `KEY=*` require only that the key is present. Keys the route does not name are ignored, so a route without conditions matches exactly what it matches today. For views, a condition counts toward specificity as the path segment it mirrors (a literal value scores like a literal segment, 3; `:name` like `:param`, 2; `*` like `*`, 1), so `/profile?section=gift-cards` wins over `/profile` on that section and `/profile` keeps every other URL at its path.

## Motivation

[Route matching](../v1/schema.md#route-matching) reads the path alone: "Query string and fragment are ignored." That is right for most query parameters (tracking, sort order, filters, pagination), which change a page's state, not what the page is. But some applications put the screen itself in the query string, and a sightmap of them cannot give those screens their own views.

**One path, several screens.** An account page at `/en-us/profile` shows a menu; `?section=gift-cards`, `?section=order-again`, `?section=order-history` and `?section=payment-methods` replace it with four different screens, each with its own components. An author can write five views, but every URL resolves to `/en-us/profile`, so the four section views are dead and their components never match. A corpus generator that derives views from captured pages hit this directly: it told the sections apart by structure and emitted `route: /en-us/profile?section=gift-cards` and its siblings, and the reference matcher resolved all 9 captures taken at those four URLs to the menu view. A consumer was strictly worse off than with one view, since the components it was shown came from the wrong screen.

**One path, many templates.** A content system serves every page from `/page?id=N`: some ids are articles, others landing pages, with nothing in common below the header. Without a query condition the corpus has one view, `/page`, and either merges both templates' components into it or covers neither well. With conditions an author can write `/page?id=101` and `/page?id=206` for the pages that matter, or `/page?id=:id` as a view for "any page" that still outranks a broader `/:section`.

**Requests already need it.** The community atlas corpora vendored under `web/src/data/atlas/` contain request routes like `/graphql?operationName=Items`, `/_graphql?body=*`, `/issues/viewer?queryId=*` and `/:owner/:repo/graphs/participation?type=sparkline*`. Their authors wanted exactly this: one GraphQL endpoint, named per operation. Today the `?` is a literal character in the last path segment, so those routes match nothing at all.

The workaround, folding each query-selected screen into its path's view and exposing the parameter as a [URL property](../v1/schema.md#url-properties), keeps the corpus valid but loses what makes a view a view: a consumer can't tell which components belong to which screen, and coverage and tags can't be scoped to it.

## Proposal

### Shape

The route string gains an optional query part. No field is added.

```yaml
# Before: the section views are dead; every URL at the path resolves to ProfileMenu.
views:
  - name: ProfileMenu
    route: /en-us/profile
  - name: GiftCards
    route: /en-us/profile?section=gift-cards     # matches nothing

# After
views:
  - name: ProfileMenu
    route: /en-us/profile
  - name: GiftCards
    route: /en-us/profile?section=gift-cards     # section's first value is gift-cards
  - name: Document
    route: /page?id=:id                          # id present, any value
  - name: Library
    route: /:org/library?type=metrics&view=*     # both conditions hold

requests:
  - name: GraphQLItems
    route: /graphql?operationName=Items
```

**JSON Schema diff.**

- `$defs.view.properties.route`: add `pattern: "^[^?]*(\\?[^&=]+=[^&]+(&[^&=]+=[^&]+)*)?$"`; extend the description to name query conditions.
- `$defs.request.properties.route`: the same `pattern` and description change.

The pattern admits any route without `?` unchanged, and after a `?` requires one or more `KEY=VALUE` pairs joined by `&`, each with a non-empty key and value. Repeated keys and malformed percent-escapes cannot be expressed in the pattern and are left to validation.

**Match inputs and expected results.** Given the views above plus `Page` at `/en-us/:page` and `Article` at `/page?id=101`:

| URL | View | Why |
|---|---|---|
| `/en-us/profile?section=gift-cards` | `GiftCards` | 3+3+3 beats `ProfileMenu` 3+3 and `Page` 3+2 |
| `/en-us/profile?utm_source=mail&section=gift-cards` | `GiftCards` | keys the route doesn't name are ignored |
| `/en-us/profile?section=order-history` | `ProfileMenu` | no condition holds; the bare path takes it |
| `/en-us/profile?section=order-history&section=gift-cards` | `ProfileMenu` | only the first value counts |
| `/en-us/profile` | `ProfileMenu` | a condition needs its key |
| `/en-us/menu?section=gift-cards` | `Page` | the path must still match |
| `/page?id=101` | `Article` | a literal value (3) beats `:name` (2) |
| `/page?id=206` | `Document` | |
| `/acme/library?view=grid&type=metrics` | `Library` | conditions hold in any order |

| Request URL | Requests | |
|---|---|---|
| `/graphql?operationName=Items` | `GraphQL`, `GraphQLItems` | with a path-only `GraphQL` at `/graphql`: all matches apply |
| `/graphql?operationName=Cart` | `GraphQL` | |

### Semantics

**Syntax.** A route is a path pattern, optionally followed by `?` and one or more conditions joined by `&`:

```
route     = path [ "?" condition *( "&" condition ) ]
condition = key "=" value
value     = literal | ":" name | "*" | literal-with-stars
```

The path part ends at the first `?` and keeps every existing rule (globs, `:param`, trailing-slash normalization). `key` and `value` are written percent-encoded, as they would appear in a URL. A `*` in a value is read before decoding, so `%2A` is a literal star; a value that starts with a literal `:` is written `%3A`. A key may be named once per route; conditions may appear in any order and mean the same.

**Matching.** A URL matches a route when its path matches the path part and every condition holds. A condition reads the **first** value of its key from the URL's query string, decoded as [`url.query`](../v1/schema.md#url-properties) already reads it (percent-decoding, `+` as a space), and compares keys and values case-sensitively:

| Condition | Holds when |
|---|---|
| `KEY=VALUE` | the key is present and its first value equals `VALUE`, decoded |
| `KEY=VAL*` | each `*` run inside the value matches any run of characters; the rest compares literally |
| `KEY=:name` | the key is present, with any value (an empty one included, as in `?id=` or `?id`) |
| `KEY=*` | the same as `KEY=:name` |

Keys the route does not name are never read: extra parameters (tracking, UI state) never stop a match, and a route with no conditions matches whatever the query string holds. The fragment stays ignored.

**A condition is a retention request.** A consumer that scrubs captured URLs SHOULD retain the query keys a corpus's routes name, as it does for [declared URL properties](0008-url-properties.md), and SHOULD surface the case where it dropped one anyway. Naming a key is not a privacy grant: a consumer MUST NOT retain a value its own policy, or [`privacy`](../v1/schema.md#privacy), would withhold. Without this, a scrubber that strips `section` makes a captured `/en-us/profile?section=gift-cards` resolve to `ProfileMenu`, silently.

A condition's value is matched like a path segment on purpose: a literal, a wildcard inside text, a named parameter, a bare wildcard. An author who knows the path grammar knows this one.

**Specificity.** A view's score is its path score plus a score per condition, each condition scoring as the segment it mirrors:

| Segment | Query condition | Score |
|---|---|---|
| Literal | `KEY=VALUE`, including a value with a `*` inside it | 3 |
| `:param` | `KEY=:name` | 2 |
| `*` | `KEY=*` | 1 |
| `**` or empty | | 0 |

Ties still go to the first-declared view, and are still reported (`TiedViews` in the reference implementation, `[Conflicts]` in its CLI).

Consequences, all following from the sum:

- A route with conditions always outranks the same path without them (its score is the path's plus at least 1). `/en-us/profile?section=gift-cards` (9) takes that section; `/en-us/profile` (6) keeps the bare page and every other section.
- Against a different path the scores just add. `/users/*?tab=billing` (3+1+3 = 7) beats `/users/admin` (6) on `/users/admin?tab=billing`. `/users/*?tab=:t` (6) ties `/users/admin` there and falls to declaration order.
- `/page?id=101` (6) beats `/page?id=:id` (5), which beats `/page` (3).

A literal query value scores like a literal path segment because it is the same kind of evidence: one more fixed part of the URL that the page had to have. Treating the query as a tie-break below the path (path score first, conditions only between equal paths) was considered and rejected; see Alternatives.

**Requests.** The same grammar and matching apply to request routes. Requests have no specificity: every request whose path, conditions and `method` match applies, as today. Express-style `:param` path segments still normalize to `*`.

**URL properties.** A `KEY=:name` condition **binds nothing**. A path `:name` binds its segment ([SEP-0008](0008-url-properties.md)); a query value is already reachable by name with `from: url.query`, `path: KEY`, and binding it as well would need rules this SEP does not want to settle (whether `name` or `KEY` is the property, renaming, collisions with path bindings). So `/page?id=:id` produces no property `id`; declare `{ name: id, extract: { from: url.query, path: id } }` if you want one. See Open questions.

**Tags.** [Tags](../v1/schema.md#tags) on views are a union across every view whose route matches the URL. Conditions apply there too: `GiftCards`'s tags reach `?section=gift-cards` URLs and no others, while `ProfileMenu`'s reach every URL at the path, as before.

**Validation.** A query part that is empty (`/a?`), has a condition without `=` or with an empty key or value, names a key twice, or contains a malformed percent-escape is invalid. The schema pattern rejects the first three; a validator reports all of them (`route-query-invalid`, an error in the reference implementation). A route with an invalid query part matches nothing. `route-conflict`, which warns when two views share a route, compares routes with their conditions in a canonical order, so `/a?x=1&y=2` and `/a?y=2&x=1` are one route; routes differing only in a condition's value are different routes. Conditions are compared as spelled, so `?q=a+b` and `?q=a%20b`, or `?id=:id` and `?id=:other`, are different routes although they match the same URLs, as `/:a` and `/:b` are.

### Conformance

A conforming SDK:

- SHOULD retain, in captured URLs, the query keys its corpus's routes name, and SHOULD surface a dropped one (see Matching).
- MUST read a route's path part up to the first `?` and match it exactly as before.
- MUST treat everything after the first `?` as query conditions, and MUST match a URL only when every condition holds against the first value of its key, decoded as `url.query` decodes it, comparing keys and values case-sensitively.
- MUST ignore query keys a route does not name, and MUST match a route without conditions regardless of the URL's query string.
- MUST add to a view's specificity 3 for each `KEY=VALUE` condition (with or without a `*` inside the value), 2 for each `KEY=:name`, and 1 for each `KEY=*`.
- MUST apply conditions to request routes as a filter, alongside `method`.
- MUST reject a route whose query part does not validate (empty, malformed condition, repeated key, malformed percent-escape), and MUST NOT match a URL against it.
- MUST NOT bind a property for a `KEY=:name` condition.

Fixtures: `039-route-query-conditions` (the match tables above, and a request filtered by a condition) and `040-route-query-invalid` (a repeated key and a malformed escape → `route-query-invalid`; one route with its conditions reordered → `route-conflict`) and `041-route-query-order` (URL, condition and declaration order never change the match).

## Alternatives considered

- **Do nothing; fold query-selected screens into one view with a URL property.** Valid today, and the right call when a parameter only changes state. For a parameter that changes the screen it hides which components belong to which screen, and a generator that can tell the screens apart has nowhere to put what it knows.
- **Path specificity first, query conditions as a tie-break.** Compare path scores; consult conditions only between equal paths. The two orderings agree whenever a route with conditions has a path at least as specific as its rival's, which covers every motivating case (a condition on the same path always wins under both). They differ when a wildcard path with conditions meets a more literal path without them: on `/users/admin?tab=billing`, the tie-break ordering picks `/users/admin` (path 6 against 4) and the sum picks `/users/*?tab=billing` (7 against 6). The sum was chosen because it is one rule over every fixed part of the URL, it needs no second comparison to explain, and it lets an author who wrote a condition for exactly that parameter win on it. The cost is that the two ways of being specific trade off against each other, which the declaration-order tie and its report make visible when they balance.
- **Declare, then reference.** Keep routes pure paths and let a view select on a *declared* URL property, so the owner extracts a query key once and views can only use it, never read a raw key (the layering [SEP-0008](0008-url-properties.md) properties follow):

  ```yaml
  urlProperties:                                  # hypothetical corpus-level layer
    - name: section
      extract: { from: url.query, path: section }

  views:
    - name: ProfileMenu
      route: /en-us/profile
    - name: GiftCards
      route: /en-us/profile
      when: { section: gift-cards }               # may only name a declared property
    - name: Bad
      route: /en-us/profile
      when: { tab: billing }                      # error: `tab` is not declared
  ```

  Fully layered, and the declaration is also the retention request. Not chosen here because a view's properties are read from the *matched* URL, so using them to pick the view is circular; it needs a corpus-level properties layer that does not exist, and a specificity rule for `when`, which is not a path segment. It is the natural follow-up if the layering matters more than the address staying in one string; the retention rule above closes most of the practical gap in the meantime.
- **A separate `query:` map on the view** (`query: { section: gift-cards }`). Explicit, and it keeps `route` a pure path pattern. Rejected because it splits one address across two fields, needs the same grammar for values anyway, and does not match how URLs are written, read, or pasted from a browser; the atlas authors reached for `?` unprompted.
- **Only `KEY=VALUE` and `KEY=:name`, no `*`.** Smaller, and enough for the motivating view cases. But `KEY=*` and `*` inside values are what existing request routes already use, and leaving them out would make those routes match a literal `*` now and change meaning if a later SEP added wildcards. Mirroring the path segment grammar costs one table row each.
- **Any value of a repeated key may satisfy a condition**, rather than the first. Closer to how some servers read `?tag=a&tag=b`. Rejected for now because `url.query` already resolves to the first occurrence, and matching and reading should agree on which value a key has; see Open questions.

## Migration

No existing route changes meaning unless it contains `?`.

- **Routes without `?`** match exactly the URLs they matched before and score the same.
- **Routes with `?`** were schema-valid but matched nothing in practice: the `?` was a literal character in the last path segment, which a URL path cannot contain except as a decoded `%3F`. Under this SEP they become conditions and start matching. The atlas request routes named above are the known cases, and their authors' intent is what this SEP gives them. A route whose query part is malformed (`/a?b`, `/a?b=`) becomes a schema and validation error instead of a silent non-match.
- **SDKs** must read the query string where they previously discarded it. The reference implementation reads it in `ViewForURL`, `TiedViews`, `TagsForURL`, `RequestsForURL` and `MatchRoute` (whose path argument may now carry a query string), and its browser overlay passes `location.search` along with the path.

## Open questions

- **Binding.** Should `KEY=:name` bind the value as a property `name`, as a path `:name` does? It reads naturally (`/page?id=:id` gives `id`), but needs rules for renaming, for a name that collides with a path binding, and for request reserved names. This SEP binds nothing so that a later one can decide.
- **Repeated keys.** `?tab=a&tab=b`: first value only (this SEP), any value, or every value? First matches `url.query`; any-value matches some server frameworks.
- **Encoding.** Values compare after `url.query` decoding, where `+` is a space. Some applications put a literal `+` in an unencoded query value; this SEP follows the form-decoding rule for consistency with `url.query` rather than per-application behaviour.
- **Case.** Keys and values are case-sensitive, as paths and `url.query` keys are. Some applications treat `?Section=Gift-Cards` as `?section=gift-cards`; a case-insensitive form (a flag, or a per-condition marker) is left out.
- **Absence.** There is no way to say "this key must be absent" (`/profile` only when `section` is missing). The bare path plus specificity covers the cases seen so far.
- **Fragments.** Hash-routed applications put the screen in the fragment, which stays ignored:

  ```
  https://app.example.com/#/settings
  https://app.example.com/#/settings/billing?tab=invoices
  ```

  Both resolve by their path (`/`), so one view covers every screen. The `?tab=invoices` in the second URL is inside the fragment, so query conditions never see it. Giving those screens views needs a fragment condition, for example:

  ```yaml
  views:
    - name: Home
      route: /
    - name: Settings
      route: /#/settings          # hypothetical syntax
    - name: Billing
      route: /#/settings/billing
  ```

  That is a separate SEP: it reads another undeclared part of the URL (the layering question above applies again), and needs rules for scoring a fragment path and for a query inside the fragment.

## References

- [Route matching](../v1/schema.md#route-matching) and [View matching: most specific wins](../v1/schema.md#view-matching-most-specific-wins): the rules this SEP extends.
- [SEP-0008](0008-url-properties.md): `url.query` decoding and first-occurrence rule, reused here; one URL grammar for views and requests.
- [SEP-0023](0023-most-specific-definition-wins.md): most-specific-wins for component definitions, the same principle applied to selectors.
- [URLPattern](https://urlpattern.spec.whatwg.org/): the web platform's URL matcher also matches the search component, as a pattern over the whole string rather than per key.
