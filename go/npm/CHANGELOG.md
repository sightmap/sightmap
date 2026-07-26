# @sightmap/sightmap

## 0.15.2

### Patch Changes

- 4c175e8: Make corpus loading robust against two malformed inputs that previously failed badly:

  - A circular `$ref` chain (`A → B → A`, or a component that references itself) sent every corpus-loading command into infinite recursion and hung the process. Loading now detects the cycle, stops expanding, and `validate` reports it as a `ref-circular` error instead of hanging.
  - `splitSelectors` only balanced parentheses, so a comma inside an attribute selector or quoted string (`[data-x="a,b"]`) was wrongly treated as a selector-list separator and split into two dead alternatives. Splitting is now aware of `[]` brackets and quoted strings (with backslash escapes), matching how CSS is actually written.

- 27cb85a: Fix view route matching so it behaves as the spec documents. Trailing slashes are now normalized off both the route pattern and the URL path before matching, so a slash-free route like `/*/projects` matches the `/acme/projects/` paths that Django, Rails, and many React-Router apps emit — previously such URLs matched no view at all, silently dropping the `[View:]` header, view memory, and view-scoped components. Express-style `:param` segments in view routes now match a single path segment (and score between a literal and `*` for specificity). And `Corpus.ViewForURL` now returns the _most specific_ matching view — using declaration order only to break equal-specificity ties — instead of the first match in corpus order, closing a divergence between the exported library and the spec.

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
