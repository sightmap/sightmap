---
"@sightmap/sightmap": patch
---

`browser fill` no longer mangles `<input type=date>` (typing `2027-04-16` into a date holding `2027-04-09` left `270416-04-09`). For `date`, `time`, `datetime-local`, `month`, `week`, `color` and `range` inputs it sets the value with the native setter, fires `input` and `change`, and reads it back, in iframes too; text inputs still take keystrokes.

`click`, `fill`, `hover`, `keypress`, `scroll` and `drag` on a background tab now bring it to the front first (`brought tab X to front` on stderr), because Chrome is reported to hit-test and focus only on the visible tab.

`fill` errors when its focusing click left focus on the page (`clicking "N" did not focus it`), and `fill --clear` reports `the field did not take focus` instead of a raw `JS exception: Uncaught`.
