---
sep: 0005
title: Request property extraction via `properties[]`
author: Clint Ayres (@jurassix)
status: Draft
created: 2026-07-31
updated: 2026-08-04
spec-version-target: 1
related-issues: [157]
related-discussions: []
---

## Summary

Add an optional `properties: RequestProperty[]` field to `Request` entries. Each property declares a named value extracted from a live request's own request/response body or headers — `field` for a dot-path into parsed JSON or a named header, `pattern` for a regex against raw body text. It is the request-side analogue of SEP-0003's DOM `properties[]`: SEP-0003 answers "what state is this element in," this answers "what does this endpoint's traffic actually say."

## Motivation

A `Request` entry today names an endpoint and optionally documents the *shape* of its payload (`request.fields[]`/`response.fields[]` — see [Payload](../v1/schema.md#payload)), but nothing lets a consumer pull a specific value out of a *live* request to reason about it. The gap surfaces concretely in a case that looks harmless by every structured signal available today: a checkout payment endpoint returns `200 OK`, but the response body reads `{"status": "declined"}`. The HTTP status says success; the only place the real outcome lives is inside the body's own `status` field, and nothing in the spec today can name that field, let alone extract it.

Today, a `Request` entry has nothing a consumer can filter or reason about beyond its own already-structured identity (`method`, route, status code) — exactly the fields that don't distinguish `checkout.payment.declined` from `checkout.payment.approved`, since both are `200 OK` responses from the same endpoint.

Other concrete cases:

- **Silent throttling** — a `200 OK` retry response whose `X-RateLimit-Remaining` header is `0`, distinguishing a genuinely-processed retry from one the server queued without saying so in its status code.
- **Partial success** — a bulk endpoint returning `200` with a body like `{"succeeded": 8, "failed": 2}`, where "failed" only shows up if something reads the count.
- **Feature-flagged responses** — the same endpoint returning materially different bodies (`{"variant": "a"}` vs `{"variant": "b"}`) under an A/B test, useful context for attributing behavior differences that aren't bugs.

## Proposal

### Shape

A `Request` entry gains an optional `properties` array. Each entry is a `RequestProperty` object with a `name` and exactly one of `field` (a dot-path into the parsed JSON body, or a named header) or `pattern` (a regex, for body content `field`'s object-key traversal can't reach — a non-JSON body, or a value embedded in a larger string).

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
    - name: status
      field: rsp.body.status

- name: CheckoutRetryPayment
  route: /api/checkout/pay/retry
  method: POST
  properties:
    - name: rate_limit_remaining
      field: rsp.headers.X-RateLimit-Remaining   # a named header, addressed by `field`
    - name: outcome
      field: rsp.body.status

# `pattern` is for a body `field` can't traverse — here the response is
# form-encoded, so it has no JSON keys to walk.
- name: LegacyCheckoutCallback
  route: /api/checkout/callback
  method: POST
  properties:
    - name: outcome
      pattern: '(?:declined|approved|deferred)'   # the whole match is the value; see Open questions on capture groups
```

### Property field reference

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | yes | The key a consumer refers to this value by. Must be a valid identifier (`[a-z][a-z0-9_]*`), matching `componentProperty.name`'s pattern. |
| `field` | string | one of `field`/`pattern` | A rooted path: `req` or `rsp`, then either `.body.<path>` (object-key traversal into the parsed JSON body) or `.headers.<name>` (one header's value, name matched case-insensitively). Also accepts one of the already-structured request-identity names (`status`, `method`, `duration`) — see [Semantics](#semantics). |
| `pattern` | string | one of `field`/`pattern` | A regex matched against the raw text of the response body, for content `field`'s object-key traversal can't reach (a non-JSON body, or a value embedded in a larger string). |
| `transform` | string | no | Same enum as `componentProperty.transform` (SEP-0003): `first_word`, `last_word`, `first_number`, `first_dollar`, `number`, `slug`. |

Exactly one of `field`/`pattern` is required — declaring both, or neither, is a schema violation (`oneOf`, not `anyOf`).

### Semantics

#### Extraction root

`field` addresses one of two roots: `req` (the request payload) or `rsp` (the response payload), each with two children:

- `.body.<path>` — object-key traversal into the parsed JSON body.
- `.headers.<name>` — one header's value. The name is matched case-insensitively, and the value is the raw header string with no traversal below it.

`pattern` takes no root. It is matched against the raw text of the response body, for a body `field` can't traverse: one that isn't JSON, or one where the value sits inside a larger string.

`status`, `method`, and `duration` are reserved top-level names, addressing the request's own already-structured identity (HTTP status code, HTTP method, timing) rather than anything inside `req`/`rsp`. A consumer MAY reference these directly wherever a property name is expected without a `properties:` declaration. This SEP does not require a `Request` entry to declare them.

#### Live-traffic requirement

Like SEP-0003's DOM extraction, `properties:` values are extracted from a **live** request/response pair as it's observed — not from the `request:`/`response:` payload documentation, which is descriptive metadata about expected shape, not live data. A tool operating on static corpus definitions alone (a linter, a coverage report) MUST treat `properties:` as declared-but-unavailable, exactly as SEP-0003 requires for offline DOM tooling.

#### Value omission

A property value is omitted when:

- `field`'s path doesn't resolve (a missing key, or the body doesn't parse as JSON)
- `pattern` finds no match

Omission is silent, mirroring SEP-0003 exactly — no error, no warning; consumers MUST NOT treat omission as an error.

#### Relationship to `request:`/`response:` (Payload)

`properties:` and `Payload.fields[]` answer different questions and don't replace each other. `Payload.fields[]` documents the *expected shape* of a payload for a human or agent reading the corpus (`schema.md`'s existing framing: "Not exhaustive; extra fields are not rejected"); it is not enforced and this SEP does not change that. `properties:` names a specific value to *extract from live traffic*. A `field:` path commonly targets a name also documented in `Payload.fields[]`, but the two lists are independent — declaring one implies nothing about the other, and this SEP does not require or suggest keeping them in sync.

### Conformance

- MUST reject a `RequestProperty` declaring both `field` and `pattern`, or neither, as a schema violation.
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
  required: [name]
  additionalProperties: false
  oneOf:
    - required: [field]
    - required: [pattern]
  properties:
    name:
      type: string
      pattern: "^[a-z][a-z0-9_]*$"
      description: "Key a consumer refers to this value by."
    field:
      type: string
      minLength: 1
      description: "Rooted path: req|rsp, then .body.<path> (JSON object-key traversal) or .headers.<name> (one header value). See SEP-0005 §Extraction root."
    pattern:
      type: string
      minLength: 1
      description: "Regex matched against the raw text of the response body, for content field's object-key traversal cannot reach."
    transform:
      type: string
      enum: [first_word, last_word, first_number, first_dollar, number, slug]
      description: "Optional post-processing applied to the extracted string. Same vocabulary as componentProperty.transform (SEP-0003)."
```

## Alternatives considered

### 1. Extend `Payload.fields[]` (the `field` $def) with extraction directives, instead of a parallel `properties[]`

`request.request`/`request.response` already have a `fields[]` array describing expected shape; add `field`/`pattern` directives directly to that existing `$defs.field` type rather than introducing a new, structurally similar `requestProperty`.

Ruled out: `Payload.fields[]` documents *expected shape for a reader*, unenforced by design (`schema.md`: "Not exhaustive; extra fields are not rejected"). Overloading it with live-extraction semantics would force every consumer that reads `fields[]` today (documentation tooling) to also understand extraction, and would make a purely-documentary field declaration ambiguous with a live-extraction one. Keeping them separate — as SEP-0003 kept `properties:` separate from a component's other structural fields — lets `properties:` be added without touching `request:`/`response:`'s existing, unenforced meaning at all.

### 2. `field` only, no `pattern` — require all extraction targets to be valid JSON

Simpler schema: one extraction mechanism, not two.

Ruled out: real response bodies aren't reliably JSON. A form-encoded or plain-text body has no keys for `field` to traverse, and neither does a JSON body whose interesting value sits embedded inside a larger string. `pattern` is the only way to reach either.

This is the weakest of the three rejections, and worth reviewer attention. `field` covers headers as well as JSON bodies (see [Extraction root](#extraction-root)), so `pattern` now earns its place solely on non-traversable bodies. If reviewers don't see a near-term case for one, dropping `pattern` from v1 is a coherent alternative: it removes the `oneOf` constraint, the `transform`-versus-capture-group question ([Open questions](#open-questions) #2), and one of the two extraction mechanisms a consumer has to implement. Regex extraction could return in a later SEP once a real body demands it.

### 3. Do nothing — leave extraction to each consumer's own ad hoc logic

The status quo: any consumer wanting "what does this response actually say" reimplements its own field/regex matching against raw request data, independently, with no shared vocabulary.

Ruled out per Motivation.

## Migration

`properties:` is purely additive — existing `requests:` entries without the field are fully valid under the updated schema. No corpus migration is required.

Existing SDKs that encounter a `properties:` entry under a `request:` MUST treat it as an unknown field; under `additionalProperties: false` this means existing SDKs reject sightmaps using it, the same constraint SEP-0003 documented for component properties. Release playbook, matching SEP-0001/SEP-0003's pattern:

1. This SEP merges (status: Accepted).
2. `sightmap/sightmap` implements request-property extraction in its network-capture pipeline. Bumped to a minor release.
3. Consumers (e.g. Subtext) pin `github.com/sightmap/sightmap/go >= <new version>` before adding `properties:` to a `requests:` entry.

## Open questions

1. **Array indexing in `field`.** `rsp.body.items[0].status` — is index syntax in scope for v1, or is `field` restricted to plain dot-paths (object-key traversal only) until a real case demands array addressing?
2. **Capture groups in `pattern`.** Today's sketch treats `pattern` as match-or-no-match against a whole target string, with the *entire matched substring* as the extracted value. Does a real case need a capture group (`pattern: 'remaining: (\d+)'` → extract just the digits) instead of relying on `transform: number` as a second pass?
3. **Should `pattern` address targets other than the response body?** As specified it takes no root, so a non-JSON *request* body is unreachable. Adding a root would mean either a companion `target:` key or allowing `field` and `pattern` together (`field` selects, `pattern` extracts a substring from what it finds). The latter would also answer #2, since a regex applied to an already-selected value has an obvious target. Both are larger changes than this SEP needs, and neither has a case demanding it yet.
4. **This SEP does not resolve** `schema.md`'s existing open question on validating `response.fields[]`'s *shape* against real traffic (enforcement, not extraction) — that's a distinct problem (type-checking a declared shape vs. naming a value to pull out) and stays open for a future SEP.

## References

- [SEP-0003](0003-component-properties.md) — the direct DOM-side precedent this SEP mirrors; its "Alternatives considered #1" already flagged requests as the place a properties-like mechanism would eventually need its own treatment.
- [SEP-0004](0004-component-tags.md) — `tags[]` already extended from components to requests/views; same "generalize a component-only mechanism once a second entity needs it" shape as this SEP.
