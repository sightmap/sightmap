# 013-not-has-leaf-scoping

Exercises a `:not(:has())` argument — the relational pseudo-class **nested inside
`:not()`** — which requires the subject's subtree to evaluate. This is the
leaf-scoping pattern `X:not(:has(X))`: match only the innermost `X`, the one that
does not itself contain another `X`. (Drawn from the JetBlue checkout form, where
a couple of fields are wrapped in an outer `jb-form-field-container` that shares
the inner field's label; scoping to leaves gives one container per field.)

A `div` root contains `fieldA` (a `jb-form-field-container` whose only descendant
is an `input`) and `wrapper` (a `jb-form-field-container` that contains a nested
`jb-form-field-container`, `fieldB`, which in turn holds an `input`). The sightmap
uses `jb-form-field-container:not(:has(jb-form-field-container))`.

`fieldA` and `fieldB` match `FormField`: neither contains a nested
`jb-form-field-container`. `wrapper` does not match: its subtree contains
`fieldB`, so its `:has(jb-form-field-container)` argument is satisfied and
`:not()` excludes it.

A matcher that ignores the `:has()` nested inside `:not()` (evaluating the `:not`
argument by tag alone) excludes **every** `jb-form-field-container`, yielding zero
matches — the offline-vs-live divergence this fixture guards against.
