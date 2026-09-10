# 014-not-complex-descendant

Exercises a **descendant combinator inside `:not()`** — a complex selector as the
`:not()` argument (CSS Selectors Level 4), which expresses an ancestor constraint
on the subject and so requires the subject's ancestor chain to evaluate. (Drawn
from the JetBlue checkout step CTA, which must exclude the `jb-sign-in` log-in
button that is also a `jb-button-primary` within `jb-checkout`.)

A `jb-checkout` contains a `jb-sign-in` wrapping `loginBtn` (a
`button.jb-button-primary`) and a sibling `stepBtn` (also
`button.jb-button-primary`, but not under `jb-sign-in`). The sightmap uses
`jb-checkout button.jb-button-primary:not(jb-sign-in button)`.

Only `stepBtn` matches `CheckoutNextButton`. `loginBtn` is excluded because it
matches the `:not()` argument `jb-sign-in button` — it is a `button` with a
`jb-sign-in` ancestor.

A matcher that cannot parse a combinator inside `:not()` rejects this valid CSS
selector outright (a corpus that fails to validate); one that parses it but drops
the ancestor part silently includes `loginBtn`. This fixture guards both.
