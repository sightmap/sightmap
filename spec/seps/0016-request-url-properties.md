---
sep: 0016
title: Request URL properties — query parameters and bound path segments
author: Clint Ayres (@jurassix)
status: Review
created: 2026-09-29
updated: 2026-09-29
spec-version-target: 1
related-issues: [187]
related-discussions: []
---

## Summary

Let a `Request` property address the request's own URL, which [SEP-0005](0005-request-properties.md)
cannot reach today. Two additions, one per half of a URL. A new `req.query` value in the `source`
enum lets `field` name a query parameter. And a `:name` segment in a request `route` binds that
segment's decoded value as a property named `name`, exactly as [SEP-0008](0008-view-route-params.md)
proposes for view routes, so a path value needs no `source` at all. Closes issue #187.

## Motivation

SEP-0005 answers "what does this endpoint's traffic actually say" by reaching into bodies and
header blocks. It cannot reach the URL, and a large share of the values worth naming live there.
`route` matches the path *structurally* and `status`/`method`/`duration` cover the request's
identity, but nothing surfaces a URL *value* that a consumer or a [signal](0007-signals.md) can
filter on.

The values that go missing are the discriminating ones:

- **Resource identity.** `GET /api/orders/8891` resolves to a request named `GetOrder`. Which order
  is thrown away at match time, so every order fetch is indistinguishable from every other.
- **Feature flags and A/B buckets.** `?variant=b` decides which code path ran. It never reaches a
  consumer at all, so behavioral differences between cohorts cannot be attributed.
- **Pagination and cursors.** `?page=7` separates a user browsing deep into results from one who
  bounced off page one.
- **Multi-tenant scoping.** `/api/org/acme/settings`: the tenant is the single most useful
  discriminator on the request, and it sits in the path with no way to extract it.

Each of these is currently recoverable only by parsing the raw URL outside the spec entirely,
which is exactly the bespoke, drift-prone matching SEP-0005 and SEP-0007 exist to eliminate.

This gap was known when SEP-0005 landed. Issue #187 records it as deferred from the review of
PR #171, specifically so the `source` enum could grow compatibly later. This is that growth.

## Proposal

### Shape

```yaml
# Before: the corpus names the endpoint; every order fetch looks identical
- name: GetOrder
  route: /api/orders/*
  method: GET

# After: the path segment binds, and the query is addressable
- name: GetOrder
  route: /api/orders/:order_id      # binds order_id
  method: GET
  properties:
    - name: variant
      source: req.query
      field: variant                # ?variant=b
    - name: page
      source: req.query
      field: page
```

### JSON Schema

- `$defs.requestProperty.properties.source`: **add** `"req.query"` to the existing enum, which
  becomes `["req.body", "rsp.body", "req.headers", "rsp.headers", "req.query"]`.
- `$defs.requestProperty`: the existing `if`/`then` requiring `field` for a headers source widens
  to require it for `req.query` too. The condition becomes
  `{ properties: { source: { enum: ["req.headers", "rsp.headers", "req.query"] } } }`.
- No new `$defs`, no property additions, no requiredness change to any existing field. Path
  binding needs **no schema change at all**: `:name` is already valid inside `route`, which is a
  plain string.

### Semantics

#### Query parameters

`source: req.query` addresses the request URL's query string. `field` names one parameter.

- **`field` is required** for this source, as it is for a headers source. A query string with no
  named parameter has no single value to resolve to.
- **Parameter names are compared case-sensitively.** URLs are case-sensitive below the host, and
  `?ID=` and `?id=` are genuinely different parameters. This differs from a headers source, which
  matches case-insensitively because HTTP header names are defined that way.
- **Values are percent-decoded**, and `+` is decoded as a space, per `application/x-www-form-urlencoded`.
- **A repeated parameter resolves to its first occurrence.** `?tag=a&tag=b` yields `"a"`.
- **A present-but-empty parameter resolves to the empty string**, which, like every other property
  source, is omitted rather than reported. `?variant=` declares nothing.
- **`pattern` composes as it already does**, refining whatever `field` resolved.

#### Bound path segments

A `:name` segment in a request `route` matches exactly one path segment, as it does today, and
additionally binds that segment's percent-decoded value as a property named `name`.

This is a change in what `:param` *produces*, not in what it *matches*. Today
[`schema.md#route-matching`](../v1/schema.md#route-matching) normalizes `:param` to `*` for request
routes; a route that matched before matches exactly the same set of URLs after, and a route using
`*` is unaffected. Only the binding is new.

- **The bound name must be a valid property name**, `^[a-z][a-z0-9_]*$`, matching
  `requestProperty.name`.
- **A bound name MUST NOT collide** with a declared `properties[]` entry, or with a reserved
  identity name (`status`, `method`, `duration`). Either is a validation error rather than a silent
  precedence rule, because both readings are defensible and guessing is worse than refusing.
- **A `:name` may not repeat within one route.**
- Bindings need no `properties[]` declaration, the same way the reserved identity names need none.

### Conformance

A conforming SDK MUST:

- Accept `req.query` in `requestProperty.source`, and require `field` alongside it.
- Resolve a `req.query` property to the first occurrence of the case-sensitively named parameter,
  percent-decoded, omitting the property when the parameter is absent or empty.
- Bind each `:name` segment of a request `route` as a property carrying that segment's
  percent-decoded value.
- Report a validation error when a bound segment name collides with a declared property name or a
  reserved identity name, and when a `:name` repeats within one route.
- Continue to normalize `:param` to `*` for route *matching*, unchanged.

A conforming SDK MUST pass the shared conformance fixture `spec/conformance/023-request-url-properties.fixture/`.

## Alternatives considered

1. **A `req.path` source with a positional `field`.** `field: 2` for the third segment. Rejected:
   positional indices are unreadable at the point of use and break the moment a segment is inserted.
   The route string already names its own segments; binding them is strictly better than counting.

2. **Fold this into SEP-0008 and cover views and requests in one proposal.** Tempting, since both
   halves extract a value from a URL. Rejected on precedent and on grammar. SEP-0003 and SEP-0005
   are already separate proposals for properties on different entities, and those entities have
   genuinely different property models: a view or component uses `extract:`, a request uses
   `source`/`field`/`pattern`. A request URL property that followed SEP-0008's `extract: query:x`
   form would be the odd one out among request properties, and one combined SEP would have to force
   a single grammar onto two models that already diverge for good reasons. The two SEPs should be
   reviewed together, and each should land in the shape its own entity already uses.

3. **Address the whole query string as one value and let `pattern` pull parameters out.** Possible
   today if `source` gained a raw-query form. Rejected: it makes every author write a regex for
   something the URL already parses unambiguously, and regexes over query strings get parameter
   ordering and encoding wrong in ways that are easy to miss.

4. **Return every occurrence of a repeated parameter.** Rejected for now: no other property source
   is multi-valued, so this would make `RequestProperty` the first, and a single value covers the
   flag, cursor and identifier cases that motivate the SEP. Noted as an open question.

5. **Do nothing and let consumers parse URLs themselves.** The status quo, and the thing issue #187
   was filed against. A consumer-side parse has no structural relationship to the `Request` entity
   it is about, so the two drift the moment the declared route changes.

## Migration

No corpus migration is required. Both halves are additive.

`req.query` is a new enum value: a corpus that does not use it is unaffected. Path binding changes
only what a `:name` segment *produces*, never what it matches, so an existing route keeps matching
exactly the URLs it matched before. A route written with `*` gains nothing and loses nothing.

**Two behaviors become errors that were previously accepted**, both only for corpora that already
use `:name` on a request route: a bound name colliding with a declared property name, and the same
`:name` appearing twice in one route. Both were meaningless before, since the binding did not
exist, so anything they describe today is already an authoring mistake. A corpus hitting either is
told what to rename.

**The additive-field pin applies** as usual (see `spec/VERSIONING.md`): an older SDK rejects
`source: req.query` as an invalid enum value rather than ignoring it.

## Open questions

1. **Should a repeated parameter be addressable in full?** First-occurrence covers the motivating
   cases, but `?tag=a&tag=b` genuinely carries two values. Adding a multi-valued form later would
   make `RequestProperty` the only multi-valued property type, so it is worth deciding whether that
   is acceptable before it is needed rather than after.

2. **Should the fragment be addressable?** `#section` never reaches the server, so it is absent
   from a server-side record and present in a client-side one. Left out rather than specified
   inconsistently.

3. **Should a bound path segment be distinguishable from a declared property?** Both arrive as
   named values. A consumer wanting to know which came from the route cannot tell. This is the same
   situation the reserved identity names are already in, so this SEP follows them rather than
   introducing a marker for one case.

## References

- Issue #187: the gap this closes, deferred from the PR #171 review so the `source` enum could grow
  compatibly.
- [SEP-0005](0005-request-properties.md): the `source`/`field`/`pattern` model this extends, and the
  source of the reserved identity names.
- [SEP-0008](0008-view-route-params.md): the same path-binding idea for view routes. The two should
  be reviewed together.
- [SEP-0007](0007-signals.md): the consumer of these values — a signal filters on an entity's
  declared properties, which is why a URL value that is not a property cannot be filtered on.
- [`spec/v1/schema.md#route-matching`](../v1/schema.md#route-matching): the `:param` normalization
  rule this SEP leaves unchanged for matching purposes.
