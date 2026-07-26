---
"@sightmap/sightmap": minor
---

CLI teardown: reorganize the tooling around reusable library subsystems and add browser devtools.

- Separate `snapshot` (observe: annotated tree + coverage) from `capture` (persist into a view's set); extract the `observe`, `coverage`, `viewset`, and `authoring` packages and merge the internal `Session` into `Corpus`.
- Make the `browser start` daemon the single session owner: retire `--launch` and the `browser launch` subcommand (live commands attach to a started session).
- Add `console` and `network` devtools commands, backed by a session-lifetime collector in the daemon that buffers console messages (with uncaught exceptions folded in) and network requests, with lazy response bodies.
- Add screenshot clipping to `browser screenshot` (`--component` / `--selector` / `--expand-pct`).
