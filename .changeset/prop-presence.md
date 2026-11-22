---
"@sightmap/sightmap": patch
---

`sightmap browser expect --prop NAME=` (an empty value) now means no match has the property, and `NAME!=` that every match has a non-empty one, the same reading `--query` gives an empty value. Before, `NAME=` compared with the empty string, which an omitted property never equals.
