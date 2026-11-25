---
"@sightmap/sightmap": minor
---

`sightmap browser get` and `expect` read each match's box (`left`/`x`, `top`/`y`, `width`, `height`, `right`, `bottom`, viewport CSS pixels), and `expect --prop` compares numbers: `width=720~2` (within a tolerance), `>=`, `<=`, `>`, `<`. Layout rules no longer need an eval of `getBoundingClientRect`.
