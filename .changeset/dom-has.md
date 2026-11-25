---
"@sightmap/sightmap": minor
---

Component properties can read `extract: { from: dom.has, path: ATTR }`: `"true"` when the node carries the attribute at all, even with an empty value, and `"false"` otherwise. This reads boolean attributes such as `data-selected=""`, which `dom.attr` omits because their value is empty.
