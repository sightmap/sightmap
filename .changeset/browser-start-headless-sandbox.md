---
"@sightmap/sightmap": patch
---

`browser start` now comes up out of the box on headless and sandboxed Linux hosts. With no display (`$DISPLAY`/`$WAYLAND_DISPLAY` unset) it defaults to headless instead of dying with "Missing X server or $DISPLAY", and when Chrome's launch fails because the host restricts its sandbox (unprivileged user namespaces clamped by AppArmor or a container) the error points straight at the fix (`--chrome-flag=--no-sandbox`) rather than leaving you to decode Chrome's stderr.
