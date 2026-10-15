---
"@sightmap/sightmap": minor
---

New on `match.Matcher`: `PrivacyForChain` and `WatchedForChain` resolve an element from its ancestor chain alone, exactly as the full tree would, since every selector in the `capture-baseline` profile depends only on an element and its ancestors; `PrivacyAttributes` lists the attributes a chain consumer must record; and the `WithPrivacyRules` option adds a consumer's own `block` and `mask` rules (a consumer `unmask` is ignored).
