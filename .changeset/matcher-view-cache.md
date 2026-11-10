---
"@sightmap/sightmap": patch
---

match: a Matcher compiles its queries once per view rather than once per page URL, so a long-lived matcher that sees many URLs of the same views holds far less (`sightmap match` over 292 Fullstory captures: peak RSS 618 MB to 189 MB, identical output). `Corpus.ViewIndexForURL` names the view a URL selects by index. Selector specificity no longer allocates per compound pair.
