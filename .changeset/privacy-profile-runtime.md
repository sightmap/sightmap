---
"@sightmap/sightmap": minor
---

Resolve privacy and watch rules outside the `capture-baseline` profile as SEP-0018 specifies: a privacy rule the profile rejects, or one whose subject matches nearly every element (`*`, `:not(.x)`), never matches loosely. A `block` or `mask` covers the whole document, an `unmask` is ignored, and a watch rule outside the profile is not reported.

New on `match.Matcher`: `PrivacyForChain` and `WatchedForChain` resolve an element from its ancestor chain alone, exactly as the full tree would; `PrivacyAttributes` lists the attributes a chain consumer must record; and the `WithPrivacyRules` option adds a consumer's own `block` and `mask` rules (a consumer `unmask` is ignored).
