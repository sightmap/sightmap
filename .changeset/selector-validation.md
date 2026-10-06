---
"@sightmap/sightmap": minor
---

Validate every component selector against SEP-0018, after flattening, so a child's selector is checked with its parents'. A selector must be chain-evaluable (`selector-not-chain-evaluable`: `:has()`, sibling combinators, positional and state pseudo-classes), in the `capture-baseline` profile (`selector-not-in-profile`: `:is()`, `:where()`, `:not()` with a list or compound, attribute flags, and so on), and must not select nearly every element (`selector-universal-subject`: `*`, `.card > *`, `:not(.x)`). These are errors on a component that declares `privacy` or `watch`, and warnings on any other component for the deprecation window. An `unmask` on a bare type selector (`body`) warns with `privacy-unmask-broad`. A selector rejected for a sibling combinator or positional pseudo-class now carries the `selector-not-chain-evaluable` code on its parse error.
