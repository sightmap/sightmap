---
"@sightmap/sightmap": minor
---

`sightmap browser expect` takes list conditions on one property across the matches, in document order: `--seq NAME=a,b,c` (exactly this list), `--set` (exactly these, any order), `--contains` (each present) and `--order` (these, in this relative order). A failure prints the list it saw and, for `--contains`/`--order`, which values are missing.
