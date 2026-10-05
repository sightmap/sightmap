---
"@sightmap/sightmap": patch
---

SEP-0014: a consumer may fall back to a view's `url:` host, but only for a corpus that declares no environments, so a corpus that predates environments keeps compiling as it did. Once a corpus declares one environment, every host comes from `origins`, and `url:` still never defines an environment or origin.
