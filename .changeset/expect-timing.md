---
"@sightmap/sightmap": minor
---

`sightmap browser expect` asserts timing: `--for MS` (the conditions hold now and keep holding, sampled every 100 ms, failing with when they broke) and `--not-before MS` (they must not hold before MS, and must by `--timeout-ms`). Rules such as "still unread at once, read after about a second" or "the row stays" no longer need sleeps in an eval.
