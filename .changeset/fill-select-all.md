---
"@sightmap/sightmap": patch
---

`sightmap browser fill` replaces the field's value instead of appending to it. It selects the focused input, textarea or contenteditable through the DOM (so it no longer depends on a Ctrl+A key event, which macOS Chrome does not turn into a select-all), and falls back to a Cmd/Ctrl+A chord with the key code and the `selectAll` editing command for other elements. After typing it reads the value back and errors when the old value was kept (`2026-04-102026-04-17`) as well as when the field is empty. `fill --clear` now works on textareas, selects and contenteditables (it picked the input value setter for every element and threw on a textarea) and fails when the clear fails; `EvalJSON` errors now carry the page exception's message.
