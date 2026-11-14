---
"@sightmap/sightmap": patch
---

`sightmap browser keypress` accepts Playwright-style chords (`Shift+KeyV`, `Meta+K`, `Alt+Shift+P`, `Ctrl+r`, `ControlOrMeta+Enter`) and sends a correct `key`, `code` and modifier state for them, so apps that match `KeyboardEvent.code` plus modifiers can be driven. A bare character (`c`, `5`, `.`, `?`) or code name (`KeyC`, `Digit1`) now yields both `key` and `code` (US layout); `Space`, `Home`, `End`, `PageUp`, `PageDown` and `F1`–`F12` are new named keys. Text is typed only when no Control, Meta or Alt is held.
