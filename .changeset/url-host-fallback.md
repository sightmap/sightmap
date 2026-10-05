---
"@sightmap/sightmap": patch
---

SEP-0014: a consumer may fall back to a view's `url:` host, but only for a corpus that declares no environments and no origins, so a corpus that predates both keeps compiling as it did. Once a corpus declares either, every host comes from `origins`, and `url:` still never defines an environment or origin. A page host is a web concept, so a view that runs only in native environments needs none.
