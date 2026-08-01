---
"@sightmap/sightmap": patch
---

Make `browser start` work — and fail legibly — on Linux and in root containers (the standard agent/CI environment):

- `FindChrome` now checks the sightmap-managed Chrome for Testing install (`~/.sightmap/browsers/`) on Linux and Windows, not just macOS, so the documented `browser install` → `browser start` flow works. The "no Chrome found" errors now point at `browser install`.
- `browser start` adds `--no-sandbox` automatically when running as root (Chrome refuses to start otherwise), and accepts `--chrome-flag` (repeatable) and `--chrome-binary` to override the launch.
- On a startup timeout, `browser start` now reports the resolved binary, the full argument list, and the tail of Chrome's stderr — instead of a bare "timed out waiting for CDP" that hid the real cause.
