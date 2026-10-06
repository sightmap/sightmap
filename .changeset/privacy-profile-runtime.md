---
"@sightmap/sightmap": minor
---

Resolve privacy and watch rules outside the `capture-baseline` profile as SEP-0021 specifies: a privacy rule the profile rejects, or one whose subject matches nearly every element (`:not(.x)`), never matches loosely. A `block` or `mask` covers the whole document, an `unmask` is ignored, and a watch rule outside the profile is not reported.
