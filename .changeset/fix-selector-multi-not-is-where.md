---
"@sightmap/sightmap": patch
---

Fix selector parser silently dropping repeated `:not()` / `:is()` / `:where()` pseudo-classes on a single compound selector. A second `:not()` (or a second `:is()`/`:where()`, including a mixed `:is():where()` pair) used to silently overwrite the first, so `button:not(.disabled):not(.hidden)` matched a `button.disabled` and `div:is(.foo):is(.bar)` matched `div.bar`. The parser now returns a clear error for a second occurrence, mirroring how it already rejects other out-of-scope CSS (`+`, `~`, `:nth-child()`). Single occurrences — including one `:is()`/`:where()` with multiple comma-alternatives — are unaffected, and `:has()` still allows multiple occurrences (AND-ed).
