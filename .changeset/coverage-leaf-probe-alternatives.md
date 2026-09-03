---
"@sightmap/sightmap": patch
---

`coverage`: the dead-component leaf-probe now checks every selector alternative (not just the first) when classifying a view component as a broken selector vs. genuinely absent. Components with multiple alternative selectors whose leaves differ were previously misclassified as `[Absent]` when only a later alternative's leaf was present in the DOM; they now correctly surface under `[Warnings] — selector likely broken`. Advisory only — `failures` and coverage percentage are unchanged.
