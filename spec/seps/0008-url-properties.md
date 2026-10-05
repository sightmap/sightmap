---
sep: 0008
title: URL properties on views and requests
author: Clint Ayres (@jurassix)
status: Accepted
created: 2026-08-05
updated: 2026-10-05
spec-version-target: 1
related-issues: [187]
related-discussions: []
---

## Summary

Give a `:name` segment in a `route` a binding, not just a match: `route: /shop/p/:product_id`
matches `/shop/p/12345` exactly as `/shop/p/*` does today, and additionally binds
`product_id = "12345"` as a property. Add an optional `properties[]` array reading two URL sources
with the [SEP-0017](0017-extract-object.md) extract object, `url.path` and `url.query`, so a
query-string value can be named and a bound segment can be renamed.

This applies to **views and requests alike**, with one grammar. Both entities carry a `route`, both
are matched against a URL, and the value an author wants out of that URL is the same kind of thing
in each case, so it would be perverse to spell it two ways.

## Motivation

A `route` can already glob-match a dynamic URL segment (`/shop/p/*`), but nothing captures *what
value* matched. The value is discarded at match time, on both entities, and it is usually the most
discriminating thing about the match.

**On a view**, an agent looking at a product page gets the identity `ProductDetail` and nothing
else; the `12345` that made it *that* product page is gone. Same for `/org/:org_id/settings`, where
the org ID is the single most useful discriminator on the view. And `?variant=blue` never reaches a
consumer at all, since [route matching](../v1/schema.md#route-matching) ignores the query string
entirely.

**On a request**, `GET /api/orders/8891` resolves to `GetOrder` and every order fetch looks
identical. Feature-flag buckets (`?variant=b`), pagination cursors (`?page=7`) and tenant scopes
(`/api/org/acme/settings`) are all invisible for the same reason.

**Both feed the same consumer.** A [SEP-0007](0007-signals.md) `signals:` rule filters on an
entity's declared properties, so a rule that should fire only for one org, one product, or one
variant has nothing to filter against. SEP-0007's conformance section says outright that a view
"has no extractable property at all".

`:param` segments are defined today only for request routes, where they normalize to `*`. Nothing
stops an author writing `:product_id` on a view route either, and the view specificity table
already scores `:param` as `2`. The score exists; the binding does not.

### Relationship to SEP-0005

This is not a gap in [SEP-0005](0005-request-properties.md). SEP-0005 deliberately scoped request
properties to bodies and header blocks and built `source` as a closed enum *designed to grow*.
Issue #187 records the URL case as explicitly deferred from the PR #171 review, "so the `source`
enum can grow compatibly later". This SEP is that growth, and it arrives through `extract:` rather
than through a new `source` value for the reason given under [Shape](#shape).

## Proposal

### Shape

A `:name` segment in a `route` binds that segment's percent-decoded value as a property named
`name`. An optional `properties[]` array names query-string values and renames bound segments.

```yaml
# Before: the identifiers are unrecoverable on both entities
views:
  - name: ProductDetail
    route: /shop/p/*
requests:
  - name: GetOrder
    route: /api/orders/*
    method: GET

# After: the route binds, and properties[] reaches the query string
views:
  - name: ProductDetail
    route: /shop/p/:product_id        # binds product_id
    properties:
      - name: variant
        extract: { from: url.query, path: variant }
requests:
  - name: GetOrder
    route: /api/orders/:order_id      # binds order_id
    method: GET
    properties:
      - name: variant
        extract: { from: url.query, path: variant }        # from the URL
      - name: outcome
        extract: { from: rsp.body, path: status }      # from the payload (SEP-0005)
```

A request property reads the URL or a payload through the same object, so a request may carry both
kinds side by side. A view reads only the URL.

**Why `url.*` and not `req.*`.** The `req.*`/`rsp.*` sources address a structured payload. A URL is
not a payload, and the `req`/`rsp` vocabulary means nothing to a page, so URL parts get their own
namespace, valid on views and requests alike.

### URL sources

A URL property's `extract.from` is one of two sources, and `path` is required:

- **`url.path`**, with `path: <segment_name>`: the value bound by a `:segment_name` in this entity's
  own `route`. Must resolve to a `:param` actually present in that route.
- **`url.query`**, with `path: <key>`: the first percent-decoded value of the named query-string
  parameter, matched **case-sensitively**. URLs are case-sensitive below the host, so `?ID=` and
  `?id=` are different parameters. Absent when the key is not present on the matched URL.

`pattern` refines either value as it does every source (SEP-0017). `join` is not valid.

**The inclusion test.** A URL part belongs here when a *named slice* of it is the whole answer a
consumer needs. `url.path` and `url.query` both pass: extraction alone fully resolves them, and
neither touches route matching. Two other candidates fail it, for different reasons:

- **`host` and `fragment` are deferred, not ruled out.** Both are strings an author will plausibly
  want to *parameterize* rather than extract opaquely: a subdomain-per-tenant deployment
  (`acme.app.com/settings`) wants `:tenant.app.com` in the route itself, and a hash-routed SPA
  (`#/orders/123`) wants `:param` matching *inside* the fragment, which route matching ignores
  today. Shipping a bare `extract: host` now answers only the shallow version and risks foreclosing
  the better one. See [Open questions](#open-questions).
- **`path` and `full` are ruled out**, not deferred; see
  [Alternatives considered](#alternatives-considered). Both hand back a whole-URL string, the exact
  high-cardinality value this SEP exists to let an author collapse into named parts.

### Match inputs and expected results

Given this corpus:

```yaml
views:
  - name: ProductDetail
    route: /shop/p/:product_id
    properties:
      - name: variant
        extract: { from: url.query, path: variant }
```

| URL | Matches? | Properties produced |
|---|---|---|
| `/shop/p/12345` | yes | `product_id="12345"` |
| `/shop/p/12345?variant=blue` | yes | `product_id="12345"`, `variant="blue"` |
| `/shop/p/12345?variant=blue&page=2` | yes | `product_id="12345"`, `variant="blue"` — `page` is undeclared, so it is not surfaced |
| `/shop/p/12345?VARIANT=blue` | yes | `product_id="12345"` — keys are case-sensitive, so this is a different parameter |
| `/shop/p/12345?variant=` | yes | `product_id="12345"` — present but empty resolves to nothing, and omission is silent |
| `/shop/p/12345?variant=a&variant=b` | yes | `product_id="12345"`, `variant="a"` — first occurrence |
| `/shop/p/blue%20suede` | yes | `product_id="blue suede"` — percent-decoded |
| `/shop/p/12345#reviews` | yes | `product_id="12345"` — the fragment is ignored, for matching and extraction alike |
| `/shop/p/12345/reviews` | **no** | — `:product_id` binds exactly one segment |
| `/shop/p/` | **no** | — a `:name` requires a segment to be present |

Renaming a bound segment, and a request carrying both entry shapes:

```yaml
views:
  - name: OrgSettings
    route: /org/:org_id/settings
    properties:
      - name: tenant
        extract: { from: url.path, path: org_id }      # rename the binding
requests:
  - name: GetOrder
    route: /api/orders/:order_id
    method: GET
    properties:
      - name: variant
        extract: { from: url.query, path: variant }     # from the URL
      - name: outcome
        extract: { from: rsp.body, path: status }   # from the payload
```

| URL | Properties produced |
|---|---|
| `/org/acme/settings` | `tenant="acme"`; the `url.path` entry renames the `org_id` binding, so `org_id` is not produced |
| `GET /api/orders/8891?variant=b` | `order_id="8891"`, `variant="b"`, plus `outcome` from the response body |
| `GET /api/orders/8891` | `order_id="8891"`, plus `outcome`; `variant` is absent |

A realistic URL, with several bindings and several parameters:

```yaml
views:
  - name: IssueDetail
    route: /org/:org_id/projects/:project_id/issues/:issue_id
    properties:
      - name: tab
        extract: { from: url.query, path: tab }
      - name: sort
        extract: { from: url.query, path: sort }
```

| URL | Properties produced |
|---|---|
| `https://app.acme.com/org/globex/projects/apollo/issues/4471?tab=comments&sort=-created#c-88` | `org_id="globex"`, `project_id="apollo"`, `issue_id="4471"`, `tab="comments"`, `sort="-created"`. The host and the fragment take no part: matching is path-only, and neither is addressable by this grammar |
| `/org/globex/projects/apollo/issues/4471/` | the same four minus `tab`/`sort` — a trailing slash is normalized away before matching |
| `/org/globex/projects/apollo/issues/4471?tab=comments&tab=files&unread=1` | `tab="comments"` (first occurrence); `unread` is undeclared and not surfaced |
| `/org/acme%2Fcorp/projects/apollo/issues/4471` | `org_id="acme/corp"` — a percent-encoded slash decodes into the value and does **not** split the segment |
| `/org/globex/projects/apollo/issues/4471?sort=a+b` | `sort="a b"` — `+` decodes to a space |
| `/org/globex/projects/apollo/issues/4471?sort=a%2Bb` | `sort="a+b"` — `%2B` decodes to a literal plus |
| `/org/globex/projects/apollo` | **no match** — every `:name` segment must be present |

Bindings compose with wildcards. `**` still binds nothing:

```yaml
  - name: DocsPage
    route: /docs/:version/**
```

| URL | Properties produced |
|---|---|
| `/docs/v2/guides/getting-started` | `version="v2"`. `**` spans the rest and names nothing |
| `/docs/v2` | `version="v2"` — `**` matches zero segments as readily as many, so the binding still resolves |

And a request where most of the query is undeclared:

```yaml
requests:
  - name: SearchIssues
    route: /api/org/:org_id/issues
    method: GET
    properties:
      - name: q
        extract: { from: url.query, path: q }
      - name: cursor
        extract: { from: url.query, path: cursor }
```

| URL | Properties produced |
|---|---|
| `GET /api/org/globex/issues?q=login+bug&cursor=cur_abc123&limit=50&_=1727614800` | `org_id="globex"`, `q="login bug"`, `cursor="cur_abc123"`. `limit` and the cache-buster are undeclared and never surface |
| `POST /api/org/globex/issues?q=x` | **no match** — the `method: GET` filter applies before any extraction |

Note `cursor` here yields the whole token, `cur_abc123`. Adding `pattern: '^cur_(.+)'` to its extract
yields `abc123`.

### Semantics

`:name` matching is layered on the existing glob matcher, not a new mechanism. `/shop/p/:product_id`
matches a URL identically to `/shop/p/*`, with the same specificity score of `2`, and additionally
records the segment's percent-decoded text. `**` cannot bind: it spans a variable number of
segments, so there is no single value to name.

**This changes what `:param` produces, never what it matches.** Request routes already normalize
`:param` to `*`; that is unchanged, so every existing route matches exactly the URL set it matched
before, and a route written with `*` gains and loses nothing.

#### Why the path is a template and the query is a list

These are two mechanisms for two halves of one URL, which looks inconsistent until you notice the
halves are not the same shape.

A **path is positional**. `/shop/p/12345` means what it means because `12345` is the third segment,
so the pattern has to show position, and an inline template is the only thing that does. Expressing
it as an unordered list forces positional indices (`path: 2`), which are unreadable at the point of
use and break the moment a segment is inserted.

A **query is associative**. Order carries nothing and the key carries everything, so a list keyed by
name is the natural shape. Expressing it as a template forces optionality syntax onto every
parameter (`?variant=:variant?`), because a query parameter is absent far more often than present.

The split is also forced by matching. The path *is* the match pattern, so a `:name` must live in the
route, and binding then costs nothing: the matcher has already walked that segment. The query takes
no part in matching at all, so there is no template for it to live in and it has to be declared
somewhere else.

**The cost is renaming.** To publish a bound segment under a different name an author touches two
places, the route and a `properties[]` entry, where a query parameter needs only one. That is the
visible price of the split, and it is paid only in the uncommon case.

This is also a third reason the fragment is deferred: it is a *third* shape again, used as a path by
hash routers, as an associative bag by OAuth, and as an opaque anchor everywhere else. Choosing one
mechanism for it means choosing which of those it is.

**Binding is implicit.** A `:name` needs no `properties[]` entry, the same way a request's reserved
identity names (`status`, `method`, `duration`) need none. Declaring one is the escape hatch for
renaming, and `extract` is required on any entry that is declared, which avoids a defaulting rule
for the plain case.

**Renaming and collisions.** An entry reading `from: url.path` *renames* that segment: its value
arrives under the entry's name, and the implicit `:name` is no longer produced. An entry may also
restate its own binding (`{ name: org_id, extract: { from: url.path, path: org_id } }`), which
changes nothing.

Every name an entity produces must be unique. A declared property, of any source, whose `name`
equals a `:name` binding the entity still produces is therefore an error (`route-binding-conflict`):
`{ name: org_id, extract: { from: url.query, path: org } }` beside `route: /org/:org_id` would put
two values under one name. Renaming the binding first resolves it:

```yaml
route: /org/:org_id
properties:
  - name: tenant
    extract: { from: url.path, path: org_id }   # renames the binding, freeing org_id
  - name: org_id
    extract: { from: url.query, path: org }     # no conflict
```

On a request, a bound name colliding with a **reserved identity name** is an error for the same
reason: both readings are defensible, and refusing beats guessing. A `:name` MUST NOT repeat within
one route.

**Names** must match `^[a-z][a-z0-9_]*$`, the pattern `componentProperty.name` and
`requestProperty.name` already use. That includes binding names: route matching treats any
`:segment` as a parameter, but only a segment whose name matches the pattern binds. `:orgId` and
`:org-id` still match and bind nothing, and validation warns (`route-param-unbound`) rather than
dropping the binding silently.

`url.query` reads the matched URL's query string independent of route matching; matching
continues to ignore the query string and fragment entirely.

**These resolve statically.** Both sources read a URL string, not live DOM state
([SEP-0003](0003-component-properties.md)) or live traffic ([SEP-0005](0005-request-properties.md)).
A saved session, a coverage report, or a lint pass over a URL alone can produce every declared URL
property with no live observation. That is the one respect in which these differ from both
precedents, and it is why they are useful in an offline pipeline.

**Value omission is silent**, as everywhere else: a `url.query` key that is absent, or present and
empty, yields no property rather than an error.

#### Declaring a URL property is also a retention request

A capture consumer that records URLs commonly scrubs them, dropping or eliding query parameters and
fragments by default and keeping only what an operator has allowed. Against such a consumer, a
declared URL property is not only an extraction: it is the corpus stating that the named
part must survive capture, which is the same statement an allowlist entry makes. The two are one
surface seen from opposite sides, and they can disagree.

**A consumer that scrubs URLs SHOULD retain the parts its corpus names.** A `url.query` key named by
any `properties[]` entry, and any segment bound by a `:name` in a route, is by declaration a value
the author needs. Dropping it before extraction runs makes the property permanently unresolvable.

**The failure is silent, and that is the hazard worth calling out.** If a consumer scrubs a named
parameter anyway, extraction yields nothing and omission is already the normal case, so the author
sees no error and no value. A consumer that drops a part its corpus names SHOULD surface that
rather than let it disappear. Note this differs from the usual omission case: "the parameter was
not on the URL" and "the parameter was removed before I looked" are different facts, and only the
second is a misconfiguration.

**Naming a part is not a privacy grant.** `{ from: url.query, path: email }` asks for a value; it does not
authorize retaining one. A consumer MUST NOT treat a declaration as permission to keep something
its own policy, or [SEP-0009](0009-component-privacy.md), would otherwise withhold. The corpus is a
floor and not a ceiling here exactly as it is there. Authors should also note that a URL property
promotes whatever the parameter holds into a named value that flows onward into signals, so naming
a parameter that carries personal data spreads it rather than containing it.

This is also a second reason the fragment is deferred rather than shipped: it is where SPA routers
put path-like data *and* where the OAuth implicit flow puts access tokens, so a bare
`extract: fragment` would invite naming a credential.

### Conformance

A conforming SDK MUST:

- Bind each `:name` segment of a view or request `route` as a property carrying that segment's
  percent-decoded value, with no `properties[]` declaration required.
- Accept an optional `properties[]` on views, and accept `url.path` and `url.query` as request
  property sources alongside the payload sources.
- Resolve `url.path` against a `:param` present in the entity's own route, and `url.query`
  against the matched URL's query string, case-sensitively, first occurrence, percent-decoded.
- Continue to normalize `:param` to `*` for route *matching*, unchanged, on both entities.
- Treat a `url.path` entry as renaming its segment, so the implicit `:name` is not produced.
- Warn when a `:segment` name does not match `^[a-z][a-z0-9_]*$`, since it matches but binds
  nothing.
- Report a validation error when `url.path` names a segment absent from the route, when a `:name`
  repeats within one route, when a bound name collides with a reserved request identity name, and
  when a declared property's name equals a binding the entity still produces.
- Omit an unresolved property silently rather than erroring.

A conforming SDK that scrubs captured URLs SHOULD retain the parts its corpus names, SHOULD surface
the case where it dropped one anyway, and MUST NOT treat a declaration as authorization to retain
a value its own privacy policy withholds.

A conforming SDK MUST pass `spec/conformance/023-url-properties.fixture/`.

### JSON Schema

- `$defs.urlExtract`: **new**. The [SEP-0017](0017-extract-object.md) `extract` object restricted
  to `from: url.query | url.path`, with `path` required and `join` disallowed.
- `$defs.urlProperty`: **new**. `{ type: object, required: [name, extract], additionalProperties:
  false, properties: { name: { pattern: "^[a-z][a-z0-9_]*$" }, extract: { $ref:
  "#/$defs/urlExtract" } } }`.
- `$defs.view.properties.properties`: **added**, optional,
  `{ type: array, items: { $ref: "#/$defs/urlProperty" } }`.
- `$defs.requestExtract.properties.from.enum`: **widened** with `url.query` and `url.path`, which
  require `path` like a headers source. Request property items stay `{ $ref:
  "#/$defs/requestProperty" }`; the deprecated `source` key's enum is unchanged, so URL sources are
  reachable only through `extract`.
- No change to `required` anywhere.
- The Go SDK's `viewFields` allowlist gains `"properties"`.

## Alternatives considered

### 1. Explicit `properties:` only, no implicit route binding

Require every bound value, including plain path params, to be declared. Ruled out: the route
already names the param, so forcing a second declaration of the same name to bind the same value
makes the common case strictly more verbose for no added clarity.

### 2. Implicit binding only, no `properties:` array

Ship `:name` binding and nothing else. Ruled out: the query-string case is a named, already-filed
gap (#187) and would need a follow-up SEP one release later for a mechanism this one can carry now.

### 3. `extract: path` / `extract: full`

Ruled out on purpose rather than deferred: both hand back the high-cardinality whole-string value
this SEP exists to let an author collapse into named, low-cardinality parts. A corpus reaching for
`extract: full` has skipped the naming work that is the point.

### 4. Two SEPs, split by entity

An earlier revision proposed the request half separately, on the precedent that SEP-0003 and
SEP-0005 are per-entity proposals. Ruled out: that precedent splits by *what is read from* — a DOM
tree versus a payload — and here both entities read from the same thing, a URL. Splitting by entity
produced two syntaxes for one idea (a view-only URL directive and a request-only `source: req.query`),
which an author would meet side by side in a single corpus. One mechanism, one spelling, which
SEP-0017 then extended to every source.

### 5. A pattern-matching grammar, as URL scrubbing uses

URL scrubbing systems are typically far more expressive than this grammar: they match URL parts
with regular expressions, often conditionally. Against that, this SEP names one exact key or one
route segment.

Most of that difference is deliberate, because **scrubbing and extraction are duals that need
opposite things**. Scrubbing must catch everything dangerous, so a broad pattern is a feature: one
`^utm_` rule covering every UTM parameter is exactly right, and over-matching costs nothing but a
redacted value. Extraction must name *one* value, so the same `^utm_` is simply broken — it yields
N values under one property name, and a consumer filtering a signal on `utm` has no idea which it
got. Breadth is safety on one side and ambiguity on the other. The inclusion test above is that
principle applied per URL part.

Two differences are **not** explained by that, and are tracked rather than dismissed:

- **Sub-value extraction.** `?cursor=cur_abc123` is more useful as `abc123`, and `/order-8891` as
  `8891`. SEP-0017's `pattern` refinement covers this: `{ from: url.query, path: cursor, pattern:
  '^cur_(.+)' }` names one value and keeps the capture unambiguous.
- **Host conditionality.** "This parameter only on this host" is expressible there and not here.
  That belongs to [SEP-0014](0014-environments-and-origins.md), which gives views and requests a
  host vocabulary, rather than to a second pattern language in this grammar.

### 6. Do nothing

The status quo: every consumer builds its own regex or string-splitting, with no shared vocabulary
and no way for an agent to discover what is available without reading that consumer's code.

## Migration

No corpus migration is required, and nothing changes what any existing route matches.

Both additions are optional. `properties[]` is new on views. On requests it widens from one entry
shape to two, which is additive: every existing payload-shaped entry stays valid unchanged.

**Two previously-meaningless things become errors**, and only for corpora already writing `:name` on
a route: a bound name colliding with a reserved request identity name, and the same `:name`
repeating in one route. Both were inert before, since the binding did not exist, so anything they
describe today is already an authoring mistake.

**The additive-field pin applies** (see `spec/VERSIONING.md`): an older SDK rejects a view
`properties:` as an unexpected property rather than ignoring it.

## Open questions

1. **`host` and `fragment`**, per the inclusion test. Both want parameterizing rather than
   extracting, and both would change route matching, so each likely deserves its own SEP. The
   fragment carries a second reason to wait: it is a common home for access tokens.
2. **Should the retention request be normative rather than a SHOULD?** A consumer that scrubs a
   named parameter renders the declaration dead, and a MUST would close that. It is a SHOULD here
   because a consumer's scrubbing policy may be a compliance obligation it cannot let a corpus
   override.
3. **Should a repeated query parameter be addressable in full?** First-occurrence covers the flag,
   cursor and identifier cases. Enabling SEP-0017's `join` on `url.query` would collect every
   occurrence into one value; whether to is worth deciding deliberately.
4. **Should a bound segment be distinguishable from a declared property?** Both arrive as named
   values. This follows the reserved identity names, which have the same ambiguity today.

## References

- Issue #187: the request-side URL case, deferred from the PR #171 review so the `source` enum
  could grow compatibly. Closed by this SEP.
- [SEP-0005](0005-request-properties.md): the payload-shaped request property this sits beside.
- [SEP-0003](0003-component-properties.md): the `properties[]`/`extract` declaration shape reused
  here.
- [SEP-0007](0007-signals.md): the consumer of these values.
- [`spec/v1/schema.md#route-matching`](../v1/schema.md#route-matching): the `:param` normalization
  rule this SEP leaves unchanged for matching.
