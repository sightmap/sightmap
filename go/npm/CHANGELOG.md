# @sightmap/sightmap

## 0.15.1

### Patch Changes

- b04eab6: Isolate concurrent browser sessions so agents working in different projects no longer cross-talk. The session file is now keyed to `--sightmap-dir` (it lives at `<sightmap-dir>/.session`) instead of a single shared `$TMPDIR` file, so a second `browser start` only ever reuses the Chrome for _its own_ corpus rather than piggybacking on another agent's browser. Free-port probing now checks the IPv4 loopback (where Chrome's CDP and the sightmap server bind), and the sightmap server binds `127.0.0.1`, so two daemons started on the default ports slide to non-overlapping ports instead of one daemon's server colliding with another's CDP port. Every session-aware `browser` command (including `console`/`network`, `status`/`stop`, `tabs`, and the low-level interactions) now accepts `--sightmap-dir` to select which session it talks to.
- 7dba2e8: Fix the Sightmap overlay getting stuck in a reload loop that flooded the captured console. The extension's version poll compared the raw `/sightmap/version` JSON text against the parsed version string, so it never matched and re-fetched every few seconds. The poll now parses the response and compares the `version` field. Also drop the post-install `chrome.runtime.reload()` step: a fresh `browser start` always relaunches Chrome with `--load-extension`, which loads the new unpacked extension directly, and the hot-reload could leave the overlay's content script uninjected until the next full restart.

## 0.15.0

### Minor Changes

- 96a1cdb: CLI teardown: reorganize the tooling around reusable library subsystems and add browser devtools.

  - Separate `snapshot` (observe: annotated tree + coverage) from `capture` (persist into a view's set); extract the `observe`, `coverage`, `viewset`, and `authoring` packages and merge the internal `Session` into `Corpus`.
  - Make the `browser start` daemon the single session owner: retire `--launch` and the `browser launch` subcommand (live commands attach to a started session).
  - Add `console` and `network` devtools commands, backed by a session-lifetime collector in the daemon that buffers console messages (with uncaught exceptions folded in) and network requests, with lazy response bodies.
  - Add screenshot clipping to `browser screenshot` (`--component` / `--selector` / `--expand-pct`).
