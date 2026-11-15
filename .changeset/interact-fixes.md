---
"@sightmap/sightmap": patch
---

`browser fill` no longer mangles `<input type=date>` (typing `2027-04-16` into a date holding `2027-04-09` left `270416-04-09`). For `date`, `time`, `datetime-local`, `month`, `week`, `color` and `range` inputs it sets the value with the native setter, fires `input` and `change`, and reads it back, in iframes too; text inputs still take keystrokes.
