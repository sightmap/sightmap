---
"@sightmap/sightmap": minor
---

Validate every component selector against SEP-0018, after flattening, so a child's selector is checked with its parents'. A selector must be in the `capture-baseline` profile (`selector-not-in-profile`: `:has()`, `:is()`, `:where()`, `:not()` with a list or compound, sibling combinators, positional and state pseudo-classes, attribute flags, and so on) and must not select nearly every element (`selector-universal-subject`: `*`, `.card > *`, `:not(.x)`). These are errors on a component that declares `privacy` or `watch`, and warnings on any other component for the deprecation window. An `unmask` on a bare type selector (`body`) warns with `privacy-unmask-broad`. A selector the general parser rejects for a sibling combinator or positional pseudo-class now carries `selector-not-in-profile` on its parse error.
