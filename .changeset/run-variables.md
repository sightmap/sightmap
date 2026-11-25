---
"@sightmap/sightmap": minor
---

`sightmap browser run` scripts can keep a value for later steps: `get Q --count --as N`, or `get Q --props FIELD --as V` for one match's field. Later steps read `$N`, which shadows the environment, and `$((...))` does integer arithmetic (`expect InboxRow --count $((N - 1))`). Steps are split when they run, so a variable set earlier expands.
