---
"@sightmap/sightmap": minor
---

`sightmap browser expect` adds `--query NAME=VALUE` for one parameter of the page URL's query string (`NAME=` means absent, `NAME!=VALUE` differs), since `--url` compares only the path, and `--no-text S` for a substring no match's text may contain.
