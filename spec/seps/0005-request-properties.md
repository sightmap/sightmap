---
sep: 0005
title: Request property extraction via `properties[]`
author: Clint Ayres (@jurassix)
status: Draft
created: 2026-07-31
updated: 2026-08-10
spec-version-target: 1
related-issues: [157]
related-discussions: []
---

## Summary

Add an optional `properties: RequestProperty[]` field to `Request` entries. Each property declares a named value extracted from a live request's own request/response body or headers: `source` names the root (a request/response body or header block), `field` names the value within it, and an optional `pattern` regex refines whatever `field` resolved. It is the request-side analogue of SEP-0003's DOM `properties[]`: SEP-0003 answers "what state is this element in," this answers "what does this endpoint's traffic actually say."

## Motivation

A `Request` entry today names an endpoint and optionally documents the *shape* of its payload (`request.fields[]`/`response.fields[]` — see [Payload](../v1/schema.md#payload)), but nothing lets a consumer pull a specific value out of a *live* request to reason about it. The gap surfaces concretely in a case that looks harmless by every structured signal available today: a checkout payment endpoint returns `200 OK`, but the response body reads `{"status": "declined"}`. The HTTP status says success; the only place the real outcome lives is inside the body's own `status` field, and nothing in the spec today can name that field, let alone extract it.

Today, a `Request` entry has nothing a consumer can filter or reason about beyond its own already-structured identity (`method`, route, status code) — exactly the fields that don't distinguish `checkout.payment.declined` from `checkout.payment.approved`, since both are `200 OK` responses from the same endpoint.

Other concrete cases:

- **Silent throttling** — a `200 OK` retry response whose `X-RateLimit-Remaining` header is `0`, distinguishing a genuinely-processed retry from one the server queued without saying so in its status code.
- **Partial success** — a bulk endpoint returning `200` with a body like `{"succeeded": 8, "failed": 2}`, where "failed" only shows up if something reads the count.
- **Feature-flagged responses** — the same endpoint returning materially different bodies (`{"variant": "a"}` vs `{"variant": "b"}`) under an A/B test, useful context for attributing behavior differences that aren't bugs.

## Proposal

### Shape

A `Request` entry gains an optional `properties` array. Each entry is a `RequestProperty` object with a `name`, a `source` naming which body or header block to read, and at least one of `field` (the value within that source) or `pattern` (a regex refinement).

```yaml
# Before — request with no property extraction
- name: CheckoutPayment
  route: /api/checkout/pay
  method: POST

# After — request with declared properties
- name: CheckoutPayment
  route: /api/checkout/pay
  method: POST
  properties:
    # JSON body value: `field` is an object-key path within `source`.
    - name: outcome
      source: rsp.body
      field: status

- name: CheckoutRetryPayment
  route: /api/checkout/pay/retry
  method: POST
  properties:
    # Header value, with a regex refinement: `field` names the header,
    # `pattern` extracts a substring from what it resolves to.
    - name: rate_limit_remaining
      source: rsp.headers
      field: X-RateLimit-Remaining
      pattern: '(\d+)'

- name: LegacyCheckoutCallback
  route: /api/checkout/callback
  method: POST
  properties:
    # No `field`: the response is form-encoded, so there's no JSON body to
    # traverse. `pattern` scans the raw source text directly.
    - name: legacy_outcome
      source: rsp.body
      pattern: '(?:declined|approved|deferred)'
```

### Property field reference

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | yes | The key a consumer refers to this value by. Must be a valid identifier (`[a-z][a-z0-9_]*`), matching `componentProperty.name`'s pattern. |
| `source` | string | yes | Which root to read from: `req.body`, `rsp.body`, `req.headers`, or `rsp.headers`. See [Extraction root](#extraction-root). |
| `field` | string | see below | The value to select within `source`. For a `.body` source, an object-key path (see [Extraction root](#extraction-root)). For a `.headers` source, a header name, matched case-insensitively; required whenever `source` is a headers source. |
| `pattern` | string | see below | An [RE2](#regular-expression-dialect) regex applied to whatever `field` resolved, or to the raw source text when `field` is absent. Capture group 1 is the extracted value when the pattern has one, otherwise the entire match. |
| `transform` | string | no | Same enum as `componentProperty.transform` (SEP-0003): `first_word`, `last_word`, `first_number`, `first_dollar`, `number`, `slug`. |

At least one of `field`/`pattern` is required (`anyOf`, not `oneOf`) — the two compose: `field` selects a value, `pattern` optionally extracts a substring from it. `source` is always required; when it names a headers source, `field` is also required — a bare regex scan across a raw header block is the addressing foot-gun this shape removes (see [Alternatives considered](#alternatives-considered)).

### Semantics

#### Extraction root

`source` is a closed set of four roots: `req.body`, `rsp.body`, `req.headers`, `rsp.headers` — no other value is valid.

- A `.body` source is the parsed JSON body. `field` walks it as a dot-separated path of object-key lookups, one segment per nesting level. Resolution dispatches on the *value's* type at each level, not the segment's: a segment against an object is a key lookup, a segment against an array is a decimal index. `field: items.0.name` resolves without any bracket syntax. One consequence: a numeric segment against an object matches an object key, not an array index — against `{"0": "x"}`, `field: 0` returns `"x"`.
- A `.headers` source is the raw header block. `field` names one header, matched case-insensitively; there is no traversal below a header value.

`pattern` always has a well-defined target, because `source` supplies one: the value `field` resolved, or the raw source text when `field` is absent.

`status`, `method`, and `duration` are reserved top-level names, addressing the request's own already-structured identity (HTTP status code, HTTP method, timing) rather than anything inside `req`/`rsp`. They sit outside `source` entirely. A consumer MAY reference these directly wherever a property name is expected without a `properties:` declaration. This SEP does not require a `Request` entry to declare them.

#### Regular expression dialect

`pattern` is a regular expression in **RE2** syntax — the dialect of Go's `regexp`, Rust's `regex`, and the `re2` npm package for JavaScript. RE2 is pinned deliberately: it matches in guaranteed linear time (no catastrophic backtracking), and because a `pattern` is validated at authoring time by one SDK and evaluated against live traffic at runtime by another, a single predictable dialect keeps the two from disagreeing about the same expression. The tradeoff is expressivity — RE2 has **no backreferences and no lookahead/lookbehind** — an acceptable loss for a value-extraction regex, and the same dialect [SEP-0006](0006-message-entity.md) pins for its `message` field.

#### Live-traffic requirement

Like SEP-0003's DOM extraction, `properties:` values are extracted from a **live** request/response pair as it's observed — not from the `request:`/`response:` payload documentation, which is descriptive metadata about expected shape, not live data. A tool operating on static corpus definitions alone (a linter, a coverage report) MUST treat `properties:` as declared-but-unavailable, exactly as SEP-0003 requires for offline DOM tooling.

#### Value omission

A property value is omitted when:

- `field`'s path doesn't resolve (a missing key, an out-of-range index, or the body doesn't parse as JSON)
- `pattern` finds no match against what `field` resolved (or against the raw source text, when `field` is absent)

Omission is silent, mirroring SEP-0003 exactly — no error, no warning; consumers MUST NOT treat omission as an error.

#### Relationship to `request:`/`response:` (Payload)

`properties:` and `Payload.fields[]` answer different questions and don't replace each other. `Payload.fields[]` documents the *expected shape* of a payload for a human or agent reading the corpus (`schema.md`'s existing framing: "Not exhaustive; extra fields are not rejected"); it is not enforced and this SEP does not change that. `properties:` names a specific value to *extract from live traffic*. A `field:` path commonly targets a name also documented in `Payload.fields[]`, but the two lists are independent — declaring one implies nothing about the other, and this SEP does not require or suggest keeping them in sync.

### Conformance

- MUST reject a `RequestProperty` with no `source`, or a `source` outside the four-value enum.
- MUST reject a `RequestProperty` declaring neither `field` nor `pattern`.
- MUST reject a `RequestProperty` whose `source` is `req.headers`/`rsp.headers` and which omits `field`.
- MUST apply `pattern` to the value `field` resolved, not to the whole source, when both are present.
- MUST use capture group 1 as the extracted value when `pattern` has one, else the entire match.
- MUST reject a `pattern` that is not a valid RE2 regular expression. Diagnostic code: `request-property-pattern-invalid` (mirrors [SEP-0006](0006-message-entity.md)'s `message-regex-invalid`).
- MUST extract `field`/`pattern` values only from live traffic; MUST NOT error when a `properties:`-declaring `Request` is used in an offline/static context — omit values instead.
- MUST omit a value silently on non-match; MUST NOT surface omission as a diagnostic.
- SHOULD apply `transform` identically to how SEP-0003 applies it for DOM properties (skip on empty/absent raw value; single transform only, not composable).

### JSON Schema diff

`$defs.request.properties.properties` gains one new optional property:

```
$defs.request.properties.properties:
  type: array
  items: { $ref: "#/$defs/requestProperty" }
  description: "Ordered list of live-traffic-value extractions, for consumers to filter or reason about."
```

New `$defs` entry:

```
$defs.requestProperty:
  type: object
  required: [name, source]
  additionalProperties: false
  anyOf:
    - required: [field]
    - required: [pattern]
  if:
    properties:
      source: { enum: [req.headers, rsp.headers] }
  then:
    required: [field]
  properties:
    name:
      type: string
      pattern: "^[a-z][a-z0-9_]*$"
      description: "Key a consumer refers to this value by."
    source:
      type: string
      enum: [req.body, rsp.body, req.headers, rsp.headers]
      description: "Which root to read from. See SEP-0005 §Extraction root."
    field:
      type: string
      minLength: 1
      description: "Value to select within `source`: an object-key path for a body source, a header name for a headers source. Required when `source` is a headers source. See SEP-0005 §Extraction root."
    pattern:
      type: string
      minLength: 1
      description: "RE2 regex (no backreferences or lookaround) applied to what `field` resolved, or to the raw source text when `field` is absent. Capture group 1 is the value if present, else the full match."
    transform:
      type: string
      enum: [first_word, last_word, first_number, first_dollar, number, slug]
      description: "Optional post-processing applied to the extracted string. Same vocabulary as componentProperty.transform (SEP-0003)."
```

## Alternatives considered

### 1. Extend `Payload.fields[]` (the `field` $def) with extraction directives, instead of a parallel `properties[]`

`request.request`/`request.response` already have a `fields[]` array describing expected shape; add `source`/`field`/`pattern` directives directly to that existing `$defs.field` type rather than introducing a new, structurally similar `requestProperty`.

Ruled out: `Payload.fields[]` documents *expected shape for a reader*, unenforced by design (`schema.md`: "Not exhaustive; extra fields are not rejected"). Overloading it with live-extraction semantics would force every consumer that reads `fields[]` today (documentation tooling) to also understand extraction, and would make a purely-documentary field declaration ambiguous with a live-extraction one. Keeping them separate — as SEP-0003 kept `properties:` separate from a component's other structural fields — lets `properties:` be added without touching `request:`/`response:`'s existing, unenforced meaning at all.

### 2. Drop `pattern` entirely — `field` only, require all extraction targets to be structurally addressable

Simpler schema: one extraction mechanism, not two.

Ruled out: real response bodies aren't reliably JSON. A form-encoded or plain-text body has no keys for `field` to traverse, and neither does a JSON body whose interesting value sits embedded inside a larger string — the `LegacyCheckoutCallback` example above has no `field` path at all. `pattern` is the only way to reach either case.

### 3. One rooted `field` string, no `source` (`field: rsp.body.status`)

Fold the root into `field` itself as a single dot-path, rather than a separate `source` key: `field: rsp.body.status`, `field: rsp.headers.X-RateLimit-Remaining`.

Ruled out per review: root and path end up sharing one string grammar, and — before headers were reachable via `field` at all — header addressing had to be smuggled into a `pattern` regex instead (`pattern: 'rsp\.headers\.X-RateLimit-Remaining:\s*(\d+)'`), which is exactly the foot-gun this SEP now removes structurally. A single string also can't express `field`+`pattern` composing on the same property, since there'd be nothing left to disambiguate "root for `field`" from "root for `pattern`" if both needed one.

### 4. Do nothing — leave extraction to each consumer's own ad hoc logic

The status quo: any consumer wanting "what does this response actually say" reimplements its own field/regex matching against raw request data, independently, with no shared vocabulary.

Ruled out per Motivation.

## Migration

`properties:` is purely additive — existing `requests:` entries without the field are fully valid under the updated schema. No corpus migration is required.

Existing SDKs that encounter a `properties:` entry under a `request:` MUST treat it as an unknown field; under `additionalProperties: false` this means existing SDKs reject sightmaps using it, the same constraint SEP-0003 documented for component properties. Release playbook, matching SEP-0001/SEP-0003's pattern:

1. This SEP merges (status: Accepted).
2. `sightmap/sightmap` implements request-property extraction in its network-capture pipeline. Bumped to a minor release.
3. Consumers pin `github.com/sightmap/sightmap/go >= <new version>` before adding `properties:` to a `requests:` entry.

## Open questions

1. **This SEP does not resolve** `schema.md`'s existing open question on validating `response.fields[]`'s *shape* against real traffic (enforcement, not extraction) — that's a distinct problem (type-checking a declared shape vs. naming a value to pull out) and stays open for a future SEP.
2. **Dotted JSON keys in `field`.** A dot-separated path can't address a JSON key that itself contains a dot — `field: flags.checkout.new_flow` against `{"flags": {"checkout.new_flow": true}}` splits into three segments and misses, silently, at the second one. One option: let `field` accept a segment array (`field: [flags, "checkout.new_flow"]`) as an escape hatch alongside the dot-string form, so an author who hits this can opt into explicit segments instead of escaping syntax. Fullstory's own equivalent (`NetworkBodySelection.path`, a `repeated string` of pre-split segments — no dot-string at all) took the array-only route for exactly this reason, and has a passing test asserting a literal `"meta.version"` key resolves correctly. A consumer lowering this SEP's `field` into that type would lose expressiveness the backend already supports if `field` stays string-only.
3. **Does `pattern` subsume `transform:`?** A regex with a capture group generalizes every case SEP-0003's fixed `transform:` enum handles (`first_number` ⊂ `pattern: '(\d[\d,.]*)'`, etc.), which raises whether the enum could be retired in favor of `pattern` alone. That's a change to SEP-0003 (already Accepted and implemented), not this SEP, and belongs in its own proposal. Worth noting the case is already half-proven in the implementation: `sightmap`'s Go `ApplyTransform` (`go/sightmap/property.go`) implements an undocumented seventh transform, `match:REGEX`, with capture-group-1 semantics, mirrored in `go/observe/properties.go` and both extension extractors and covered by `go/sightmap/property_test.go` — but it's absent from both SEP-0003's prose and `sightmap.schema.json`'s `transform` enum, so the Go loader currently accepts syntax the schema would reject.
4. **Addressing URL path/query components.** `source` covers request/response bodies and header blocks, but not the request URL's own path segments or query parameters — there is no way to extract, say, a `?variant=` query value or an `/orders/:id` path-segment value as a property. `route` matches the path structurally and `status`/`method`/`duration` cover identity, but neither surfaces a query/path *value*. Deferred from v1; tracked in [issue #187](https://github.com/sightmap/sightmap/issues/187), so the `source` enum can grow compatibly later.

## References

- [SEP-0003](0003-component-properties.md) — the direct DOM-side precedent this SEP mirrors; its "Alternatives considered #1" already flagged requests as the place a properties-like mechanism would eventually need its own treatment.
- [SEP-0004](0004-component-tags.md) — `tags[]` already extended from components to requests/views; same "generalize a component-only mechanism once a second entity needs it" shape as this SEP.
- [Issue #157](https://github.com/sightmap/sightmap/issues/157#issuecomment-5197619464) — review feedback that motivated the `source`/`field`/`pattern` split in this revision.
