---
"@sightmap/sightmap": minor
---

Add URL properties on views and requests (SEP-0008).

A `:name` segment in a view or request `route` now **binds** that segment's percent-decoded value as a property, needing no declaration. An optional `properties[]` array reads with the SEP-0017 extract object, `from: url.query` or `from: url.path`, for naming a query-string value or renaming a bound segment.

One grammar covers both entities. Both carry a `route`, both are matched against a URL, and the value an author wants out of that URL is the same kind of thing in each case. A request property reads `url.query`/`url.path` alongside its body and header sources, so one request may carry URL-shaped and payload-shaped properties side by side.

This changes what `:param` produces, never what it matches: route matching still normalizes `:param` to `*`, so every existing route matches exactly the URL set it matched before.

A declaration is also a **retention request**. A consumer that scrubs captured URLs should retain the parts its corpus names, since a scrubbed parameter makes the property permanently unresolvable and omission is otherwise silent. Naming a part is not a privacy grant: a consumer must not treat it as authorization to retain a value its own policy would withhold.

Removes `transform` from the proposal, which SEP-0010 deleted from every property type.
