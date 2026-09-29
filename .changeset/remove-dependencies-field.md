---
"@sightmap/sightmap": minor
---

Remove the `dependencies` field from views and components (SEP-0001, Rejected).

`dependencies` asked a curator to hand-maintain a dependency graph that static import analysis derives more accurately and for free, and `match --path`, the reverse-lookup consumer it existed to serve, was never built. No SDK ever read the field.

It is gone from `$defs.view` and `$defs.component` in the schema, from the loader's unknown-field allowlist, from the normative spec, and from conformance fixtures `008-dependencies-binding` and `108-fmt-dependencies-canonical`.

**Breaking for any corpus that carries the field.** Because the schema sets `additionalProperties: false`, a view or component with `dependencies:` is now rejected as schema-invalid rather than silently accepted. Delete the key; nothing consumed it.

`canonical-format.md` keeps its sort-and-deduplicate rule for unordered string arrays. `dependencies` was its only instance, but SEP-0014 relies on the rule for view- and request-level reference lists.
