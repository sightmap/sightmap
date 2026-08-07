---
"@sightmap/sightmap": minor
---

Add `properties:` to request definitions ([SEP-0005](https://github.com/sightmap/sightmap/blob/main/spec/seps/0005-request-properties.md)): named values a consumer extracts from a live request/response pair, so a `200 OK` whose body says a payment was declined can be reasoned about.

- `field` addresses a rooted path: `req`/`rsp`, then `.body.<path>` for JSON object-key traversal or `.headers.<name>` for one header value.
- `pattern` is a regex against the raw response body, for a body `field` cannot traverse.
- `transform` shares the component-property vocabulary.

New validation, closing a gap where the Go SDK accepted corpora the JSON Schema rejected:

- `request-property-invalid-name`, `request-property-no-extractor`, and `request-property-both-extractors` (errors) enforce in Go what only ajv enforced before.
- `request-property-shadows-reserved` (warning) fires when a property is named `status`, `method`, or `duration`, which shadows the request's HTTP identity and makes it unreachable from a signal filter.
- `field-type-invalid` (error) rejects an unquoted non-string scalar in a schema-string field. yaml.v3 decodes any scalar into a Go string by taking the raw lexeme, so `field: 200` used to load as `"200"` while ajv rejected it.

Extraction itself is not implemented: the SDK parses and validates these declarations but resolves no path and applies no transform. `spec/v1/schema.md` now marks the evaluation requirements as such.
