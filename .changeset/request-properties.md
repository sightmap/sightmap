---
"@sightmap/sightmap": minor
---

Add `properties:` to request definitions ([SEP-0005](https://github.com/sightmap/sightmap/blob/main/spec/seps/0005-request-properties.md)): named values a consumer extracts from a live request/response pair, so a `200 OK` whose body says a payment was declined can be reasoned about.

- `source` names the root (a closed enum: `req.body`/`rsp.body`/`req.headers`/`rsp.headers`).
- `field` selects a value within `source`: an object-key dot-path for a body source, a header name for a headers source (required there).
- `pattern` is an **RE2** regex (Go `regexp` / the `re2` npm package for JS; no backreferences or lookaround) that refines what `field` resolved, or scans the raw source text when `field` is absent. `field` and `pattern` compose (`anyOf`) instead of being mutually exclusive.
- `transform` shares the component-property vocabulary (unchanged; cleanup tracked separately).

New validation, closing a gap where the Go SDK accepted corpora the JSON Schema rejected:

- `request-property-invalid-name`, `request-property-no-extractor`, `request-property-source-invalid`, `request-property-headers-require-field`, and `request-property-pattern-invalid` (errors) enforce in Go what only ajv enforced before.
- `request-property-shadows-reserved` (warning) fires when a property is named `status`, `method`, or `duration`, which shadows the request's HTTP identity and makes it unreachable from a signal filter.
- `field-type-invalid` (error) rejects an unquoted non-string scalar in a schema-string field. yaml.v3 decodes any scalar into a Go string by taking the raw lexeme, so `source: 200` used to load as `"200"` while ajv rejected it.

Extraction itself is not implemented: the SDK parses and validates these declarations but resolves no `source`/`field`/`pattern` and applies no transform. `spec/v1/schema.md` marks the evaluation requirements as such, and the `018-request-properties` conformance fixture is now executed by the Go test suite.
