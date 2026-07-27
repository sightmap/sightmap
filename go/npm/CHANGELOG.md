# @sightmap/sightmap

## 0.15.7

### Patch Changes

- e1e4310: `snapshot` now surfaces runtime match conflicts in a `[Conflicts]` section — the ambiguities that only exist against a live page, complementing the static corpus-conflict warnings `validate` emits. Two triggers:

  - **A single DOM node matched by more than one distinct component name.** Matching is first-match-wins, so only one applied and the others were silently dropped — which is also why a correct-looking component can report `0 matches`. The section names the node, the competing components, and which one won.
  - **Two or more views matching the current URL at equal specificity**, where declaration order alone decided the winner.

  Adds `match.FindConflicts` and `Corpus.TiedViews`; both are computed during `observe.Page`. (The view-tie half is provisional pending the route-specificity decision in the functional-decomposition proposal.)

- d5ccb5a: `validate` now surfaces two classes of invalid corpus the loader previously dropped silently (so they shipped with a green check):

  - **Missing required fields** — a component with no `selector` (or no `name`), and a view with no `name`, are now errors (`missing-selector` / `missing-name` / `missing-route`) instead of being quietly discarded during load.
  - **Unresolved `$ref`** — a `$ref` naming a component that no file defines is now a `ref-unresolved` error (matching the spec's MUST), instead of the reference being silently skipped.

  Both exit non-zero. The spec's diagnostic-code table documents the new error codes alongside the corpus-conflict warnings.

- 8a16957: `validate` now warns on unknown fields. The typed loader silently ignored any YAML key it didn't recognize, so a typo like `memroy:` — or a half-baked field — vanished without a trace. `validate` now walks the raw YAML and emits an `unknown-field` **warning** (not an error) for any key the spec doesn't define at its position, at any nesting depth. It warns rather than rejects, so authors can stash experimental fields (e.g. `macros:`) during development; recognized fields — including the reserved tooling fields `access` and `snapshots` — are never flagged, and `.sightmap/config.yaml` is excluded. This completes strict `validate` alongside the earlier required-field and `$ref` checks.
- 5be278e: Fix `report` and make the view `url:` field first-class. `url:` is now read **per view** (with a file-level `url:` as a default for views that omit their own), instead of only at the file level. Previously per-view `url:` was silently dropped, so `report` — which needs a representative URL per view — errored with `no views with URLs found` even when every view declared one. `url:` is also now part of the published schema (SEP-accepted alongside `properties`), so a corpus that declares it validates instead of failing `additionalProperties: false`.

## 0.15.6

### Patch Changes

- 774637d: `validate` now distinguishes **errors** (which fail validation, exit 1) from **warnings** (advisory, exit 0), and warns on silent corpus conflicts that a fallback rule was resolving without telling you:

  - **`merge-collision-view`** — two or more views share a `name`.
  - **`route-conflict`** — two or more views share the same (normalized) `route`; only the first-declared applies to that URL (this is the "same-route hijack" that can silently drop a view's components).
  - **`merge-collision-component`** — two or more root-level global components share a `name` with different selectors.

  Findings now carry a stable `code` and `severity`. Scoped component name reuse (the same child under multiple parents) is correctly _not_ flagged, since that is intentional. The spec's diagnostic-code table documents the new corpus codes.

- 0ed063e: Make `snapshot` loud about corpus-match state instead of silently omitting it. When a corpus is loaded but no view's route matches the URL (e.g. an auth redirect to a login page), the output now shows a `[No view matched] <url>` notice instead of a headerless tree that looks identical to a normal snapshot — so an agent knows the page is off the map. And the `[Coverage]` summary is now printed whenever a corpus is applied, including for a matched view that has no components yet (every interactive node reads as an orphan); previously that case printed only the view header, leaving the documented `snapshot --coverage` bootstrap with nothing to iterate on.

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
