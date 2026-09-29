---
"@sightmap/sightmap": minor
---

Add request URL properties (SEP-0016, Draft): `source: req.query` and bound `:name` path segments.

SEP-0005 reaches bodies and header blocks but not the URL, so resource identifiers, feature-flag variants, pagination cursors and tenant scopes — values that usually live in the path or query — could not be named or filtered on.

`source: req.query` addresses the query string, with `field` naming one parameter. Names are compared **case-sensitively**, unlike a headers source, because URLs are case-sensitive below the host. Values are percent-decoded with `+` as a space, a repeated parameter resolves to its first occurrence, and `field` is required for the same reason it is for headers.

A `:name` segment in a request `route` now binds that segment's decoded value as a property named `name`, needing no `properties:` entry. This changes what `:param` produces, never what it matches: route matching still normalizes `:param` to `*`, so every existing route matches exactly the URLs it matched before.

Two previously-meaningless things become errors, and only for corpora already using `:name` on a request route: a bound name colliding with a declared property or reserved identity name, and the same `:name` repeating in one route.
