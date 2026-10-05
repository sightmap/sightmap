---
"@sightmap/sightmap": patch
---

SEP-0014: a consumer may fall back to a view's `url:` host when the view resolves no origin, so a corpus that predates environments keeps compiling as it did. `url:` still never defines an environment or origin.
