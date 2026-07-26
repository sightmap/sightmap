# @sightmap/sightmap

## 0.15.5

### Patch Changes

- 04c6216: Make `browser click` and `fill` fail loudly instead of silently no-op'ing. `click` now scrolls the target to the center of the viewport and verifies its center is the top-most element there before dispatching — it errors when the target can't be positioned in the viewport (previously an off-screen target below the fold was clicked at a coordinate that hit nothing and still exited 0) or is covered by another element (an open overlay/modal). The success confirmation reports the real post-scroll coordinates. `fill` now reads the value back after typing and errors when a non-empty value was typed but the field is still empty — the signature of a React-controlled input where plain typing doesn't stick — telling you to retry with `--clear`.
- 307839a: `sel-probe` now cross-checks the offline matcher. It queries the live DOM as before, but also runs the same selector through the offline matcher that `snapshot`/`coverage`/`capture` use, prints that count, and emits a `⚠ offline/live divergence` warning (with a hint) when the two disagree. This closes the false-confidence trap where a selector "verified" against the live DOM is silently dead in the corpus — e.g. attribute selectors on `id` (`[id^="…"]`) match live but never offline, because `id` is a dedicated node field rather than a matchable attribute.

## 0.15.4

### Patch Changes

- ce30028: Make browser-use feedback loud: the interaction commands (`click`, `fill`, `hover`, `keypress`, `scroll`, `drag`, `wait-for`, `dialog`, `tabs resize`) now echo a short confirmation on success instead of exiting silently — e.g. `clicked CartNavButton @ (674,30)` — so an agent driving the browser can see what happened. Several raw CDP/Go errors are now rewritten into actionable messages: a `wait-for` timeout reads `timed out after 800ms waiting for selector "..."` (not `context deadline exceeded`), resolving a dialog when none is open says so plainly, and fetching an evicted network response body explains that bodies are only retained briefly. `snapshot` now prints a note when no `.sightmap` corpus is found at the target directory instead of silently rendering an un-annotated tree.

## 0.15.3

### Patch Changes

- 7c3b990: Stop green-lighting broken corpora and blank pages:

  - `snapshot` now exits non-zero when the corpus is present but fails to parse (a missing corpus stays non-fatal — the tree still renders), and when the observed page has 0 interactive nodes (blank or still loading). It still renders whatever it observed first. `inspect` and `suggest` now warn on a bad corpus instead of silently ignoring it.
  - Coverage no longer marks a page with 0 interactive nodes as a pass. The coverage line renders a distinct `∅` mark (instead of `✓`), the `coverage` command counts empty captures as failures, and `capture` refuses to persist a blank/loading page as a view's baseline (override with `--force`).

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
