---
name: sightmap-browser
description: Use when driving a live browser session with a sightmap — starting or navigating a Chrome session, taking annotated component snapshots to read page state, and interacting with elements by component ID or component query. Activate when a .sightmap/ directory is present and you need to observe or act on the running app.
activation:
  - .sightmap/ directory exists in project
  - driving a live browser session against a sightmap corpus
---

# Sightmap browser: observe and interact

The sightmap CLI drives a Chrome-for-Testing session and layers your `.sightmap/`
corpus onto the live page, so you can **read page state** as an annotated
component tree and **act on elements by their semantic component identity**
instead of brittle CSS. To build or maintain the corpus itself, see the
`sightmap-authoring` skill.

**What to reach for:**

- **`snapshot`** — read page state as an annotated component tree (start here).
- **interaction** (`click`/`fill`/`scroll`/…) — drive the page by component identity.
- **`browser screenshot`** — visual evidence; clip to one component with `--component`.
- **`console` / `network`** — debug what the page logged and requested.
- **`inspect`** — raw DOM for authoring selectors (see `sightmap-authoring`).

## Installation

Every command below needs the `sightmap` binary on your PATH. **Check first,
install only if missing** — run `sightmap version`; if that fails, install it:

```bash
npm install -g @sightmap/sightmap                                 # recommended: global install
sightmap version                                                 # verify it's on PATH
go install github.com/sightmap/sightmap/go/cmd/sightmap@latest    # or build from source
```

This skill drives a **multi-step browser session**, so prefer a global install
over `npx @sightmap/sightmap <command>` (which re-resolves the package on every
invocation). Use `npm` in instructions — the published package is identical under
`pnpm`/`yarn`, but npm is the safe baseline.

## Session management

`browser start` is a **long-running foreground daemon**: it owns Chrome and the
console/network collector for the whole session, so it **holds the shell** until
you stop it. Every other command here is a thin client that connects to that
daemon and returns immediately.

- **Interactive:** run `start` in its own terminal, drive from another.
- **Scripts / agents** (shells that run each command to completion): use
  **`sightmap browser start --detach`**. It backgrounds the daemon in its own
  session, waits until it is serving, and returns — so it never hangs your shell,
  and (unlike `nohup start &`) the daemon survives between one-shot commands.
- A **client** command that hangs almost always means the daemon isn't up —
  run `browser status`, don't wait it out.
- On a headless Linux host `start` auto-detects the missing display and runs
  headless; if Chrome hits a sandbox restriction the error tells you to add
  `--chrome-flag=--no-sandbox`.

| Command | What it does |
|---------|-------------|
| `sightmap browser start` | Launch Chrome + the overlay server. **Foreground daemon — holds the shell; pass `--detach` in scripts/agents.** Creates `.sightmap/` if absent and writes `.sightmap/.session` (fails loudly if it can't). Every other command finds the session via this file, so with no session file a command warns and falls back to the default CDP port — which may be a **different agent's** Chrome; pass `--addr` to be explicit. |
| `sightmap browser status` | Check session health, tabs, and current URL. Reports `⚠ degraded` when Chrome's CDP is up but the daemon's HTTP server was reaped. |
| `sightmap browser navigate 'URL'` | Navigate to URL (positional arg — no `--url` flag). |
| `sightmap browser stop` | Stop Chrome session. |
| `sightmap browser eval 'js'` | Evaluate JS in page context. Returns JSON-serializable values only — DOM element references return an error. A returned promise is awaited, so `async` evals resolve to their value. |
| `sightmap browser inject --file X --persist` | Inject a script that re-runs on **every** new document/tab for the whole session (survives navigations), vs `eval`'s one-shot. `--list` / `--remove ID` manage them. Needs a running session (the daemon holds the registry). |
| `sightmap browser mcp list` | Enumerate the WebMCP tools the page exposes via `document.modelContext` (name, description, input schema; `--json` for full schemas). Reports native vs polyfilled vs absent; fails loudly with the Chrome-flag hint when absent. When a page exposes tools, prefer a named `mcp call` over blind click/fill. |
| `sightmap browser mcp call <tool> --args '{…}'` | Invoke one exposed tool via `executeTool`. Args via `--args JSON` and/or repeatable `--param k=v`. Unwraps the WebMCP `CallToolResult` (text/guidance shown as itself, not a stringified blob) and exits non-zero on tool `isError`. One tool at a point in time — no cross-navigation; if the tool navigates, re-run `mcp list` on the new view. |
| `sightmap browser screenshot --out FILE.png` | Screenshot the page. Clip to a component with `--component NAME` (or `--selector CSS`), optionally `--expand-pct N` for context; `--full-page` captures the whole scrollable page. |

## Reading the page: annotated snapshots

`sightmap snapshot --url URL` prints the page's component tree with your corpus
layered on (add `--coverage` for a terse coverage-only view that suppresses the
tree). Each line starts with a numeric component ID, then the node content:

```
42 [SiteHeader]
  83 [DesktopNavLink label="Explore"]
  91 link "Privacy Policy"
```

- **`[ComponentName prop="val"]`** — sightmap-matched node; role suppressed
- **`role "text"`** — unmatched node; role and accessible name shown
- **`42`** prefix — probe component ID; usable as a handle for interaction commands

Properties are sorted alphabetically. The accessible name is shown last inside
brackets when not suppressed (an exact match with a property value).

### Iframes

A snapshot descends into every iframe — same-origin, cross-origin (a separate
process) and nested at any depth — and hangs each frame's tree under its
`Iframe` node, so an MCP Apps widget's controls show up as ordinary components:

```
70 Iframe "destination_dates view"
  1_9 Iframe
    2_13 [DateWindows destination="seville"]
      2_26 [DateWindow start="2027-03-03" end="2027-03-06"]
```

- **Ids carry a frame prefix.** The top document's ids are bare (`42`); the Nth
  frame's are `N_…` (`2_26`). Click, fill, hover, drag, keypress, scroll, `bounds`,
  `wait-for` and `screenshot --component` all take them like any other id, and
  find the right frame. Bounds are in top-page coordinates, and a click's point
  is worked out from the live layout right before it is sent, so it lands after
  a scroll. A click refuses (`covered by another element`) when something in the
  page is drawn over the frame at that point. `drag` works the same way, but
  its source and target must be in one document or in frames of one process.
- **A frame is matched as a page of its own, at its own URL** — so the corpus
  view whose route matches the *frame's* URL (e.g. a widget host's `/sandbox`)
  supplies its components, not the page's view. A selector does not cross a
  frame boundary in either direction. A `srcdoc` / `about:blank` frame takes its
  parent's URL. A masked or blocked `<iframe>` masks or blocks its document.
- **A `[Frames]` section** lists each frame document (its root id, URL, and the
  view that URL resolved to) and then any `⚠` notes about what was left out.
  `no view matched` on a frame means the corpus has no view for the frame's
  path: add one (a widget's route is its host's sandbox path).
- **Size.** Each frame is capped at 3000 nodes and a page at 32 frames; a frame
  past the cap is cut and the `[Frames]` notes say so. `--frame-max-nodes N` and
  `--max-frames N` change the caps; `--no-frames` leaves frames out (an `Iframe`
  is a leaf, the old behaviour). Chrome gives out-of-process frames to the
  session on its own; nothing to configure.
- `keypress KEY <id-or-query>` focuses the target first (in its frame); with no
  target the key goes to whatever is focused.

The `[View: …]` header may be followed by a `tags:` line: the union of tags
across every view whose route matches the page. A component the corpus marks
`privacy: block` or `mask` renders **without** its content properties or AX
`value`. That is deliberate withholding, not a broken extract. Under `mask`,
read a control's state from a `dom.state` property (e.g. `checked`) instead of
its text; under `block` nothing resolves.

If the page has **0 interactive nodes**, coverage renders `∅` (not `✓`) and
`snapshot` exits non-zero — the page is blank or still loading. Wait for it with
`browser wait-for --selector ...` (or `--wait N`) and re-snap before acting.

## Interaction (by ID or component query)

`click`, `fill`, `hover`, and `scroll --component-id` accept **either** a numeric
probe ID from snapshot output **or** a component query (see below):

| Command | What it does |
|---------|-------------|
| `sightmap browser click <id>` | Click element by probe ID |
| `sightmap browser click 'ComponentQuery'` | Click element by sightmap identity (preferred on dynamic pages) |
| `sightmap browser fill [--clear] <id-or-query> "text"` | Replace an input's value by typing (date/time/color/range inputs are set, not typed). Errors if the value doesn't stick or the old text was kept — retry with `--clear` (see gotchas). |
| `sightmap browser hover <id-or-query>` | Hover over element |
| `sightmap browser keypress Enter [<id-or-query>]` | Press a key, or a Playwright-style chord (see [Keyboard](#keyboard)); with a target, focus it first (in whichever frame holds it) |
| `sightmap browser scroll --delta-y 500` | Scroll the page |
| `sightmap browser scroll --component-id <id-or-query>` | Scroll a component into view |
| `sightmap browser click --x N --y N` | Click raw coordinates (escape hatch; layout-fragile) |
| `sightmap browser drag <src> --to <target> [--to-position above\|below\|center]` | Pointer-drag one component onto another (see [Dragging](#dragging)) |
| `sightmap browser drag <src> --delta-x N --delta-y N` | Pointer-drag a component by a pixel offset |
| `sightmap browser wait-for --view <Name>` | Wait until the URL resolves to a sightmap view (the step boundary after a navigating action) |
| `sightmap browser wait-for --component '<Query>'` | Wait until a component query matches a node |
| `sightmap browser wait-for --selector "[data-loaded]"` | Wait until a CSS selector matches (also `--url SUBSTRING` (plain substring, not glob), `--load`) |
| `sightmap browser tabs list` | List open tabs |

### Keyboard

`keypress` takes one key or a chord, `Modifier+…+Key`, the way Playwright's `press` does:

```bash
sightmap browser keypress c               # key "c", code "KeyC"
sightmap browser keypress Shift+KeyV      # key "V", code "KeyV", shiftKey
sightmap browser keypress Meta+K          # ⌘K; no "k" is typed
sightmap browser keypress Alt+Shift+P
sightmap browser keypress ControlOrMeta+Enter   # Meta on macOS, Control elsewhere
```

- **Modifiers:** `Shift`, `Control` (`Ctrl`), `Alt` (`Option`), `Meta` (`Cmd`, `Command`),
  `ControlOrMeta`. The page gets the modifier state on keydown/keyup and key events for
  the modifiers themselves.
- **Keys:** a character (`c`, `C`, `5`, `.`, `?`, `[`), a code name (`KeyC`, `Digit1`,
  `Period`, `BracketLeft`), or `Enter`, `Tab`, `Escape`, `Space`, `Backspace`, `Delete`,
  `Home`, `End`, `PageUp`, `PageDown`, `ArrowUp/Down/Left/Right`, `F1`–`F12`. Both forms
  give the same `event.code` (US layout), so apps that match `event.code` plus modifiers
  work. A character is not a modifier: `C` is key `C` with no `shiftKey`; use
  `Shift+KeyC` or `Shift+c` for Shift held.
- **Text:** a character is typed (into a focused input) only when no Control, Meta or Alt
  is held.
- **Sequences** (`G` then `I`) are separate calls: `keypress g` then `keypress i`.

### Dragging

`drag` moves the pointer the way a person does, so pointer-event drags (boards, sortable
lists, sliders) register it: it scrolls the source into view (only if it is not already
fully visible, so a visible-to-visible drag leaves the layout alone), presses inside it, makes
`--steps N` mouse moves (default 10, about 16 ms apart, button held), and releases at the
drop point. Jumping straight to the target would never pass the app's activation distance.

```bash
# Drop WEB-143 on the top edge of the first card of another column (inserts above it).
sightmap browser drag 'IssueCard[identifier="WEB-143"]' \
  --to 'BoardColumn[group="status:in progress"] IssueCard#0' --to-position above
```

- **Source and target** are probe ids or component queries (both the same kind: resolving a
  query re-probes the page and invalidates probe ids). Queries are resolved in one pass.
- **`--to-position`:** `center` (default), `above` (4 px inside the target's top edge) or
  `below` (4 px inside its bottom edge). Use `--delta-x/--delta-y` instead of `--to` to drag
  by an offset; give one or the other.
- **Where it presses:** a point on the source that is not an interactive child (button,
  link, input, chip). Override with `--from-offset X,Y` (pixels from the source's top-left).
- **Both ends must be reachable.** Before pressing, `drag` checks that the start point is
  over the source and the drop point is over the target (`elementFromPoint`), so a target
  scrolled under a column header or a sticky bar, or outside the viewport, fails with what
  is in the way instead of dropping on nothing. Scroll the target fully into view (or
  `tabs resize` for a taller window) and retry. A source or target inside a shadow root is
  refused the same way: `document.elementFromPoint` reports the shadow host there, not the
  element (`click` has the same limit).
- It drives the pointer only: native HTML5 drag-and-drop (`draggable` + `dragstart`) is not
  started by it. Confirm the result with `eval` or a fresh `snapshot`; `drag` reports only
  that the gesture was sent.

### Component queries (CSS-shaped, over sightmap components + extracted properties)

Probe IDs are extraction-local: they're reassigned every snapshot, so on
re-rendering pages an `<id>` from an earlier call goes stale. A component query
re-resolves against the live tree atomically (extract → match → act in one pass),
so nothing goes stale. Prefer queries on dynamic pages.

| Query | Resolves to |
|-------|-------------|
| `FulfillmentTileButton[label=deliveryTile]` | the `FulfillmentTileButton` whose `label` property equals `deliveryTile` |
| `ProductCard[name^="Webber grill"]` | a `ProductCard` whose `name` starts with `Webber grill` |
| `ProductCard[name*="weber" i]` | substring, case-insensitive (`i` flag) |
| `LoginForm UserNameInput` | a `UserNameInput` anywhere under a `LoginForm` (target = last component) |
| `FulfillmentTileButton#1` | occurrence 1 (0-based) when several match — weak fallback |

- Brackets are **component names**; `[prop op value]` filters on **extracted
  properties** (not raw DOM attributes). Operators: `=` exact, `^=` prefix,
  `*=` substring, with an optional trailing ` i` for case-insensitive.
- Whitespace is a **descendant** combinator; the **last** component is the target.
  It matches at any depth (`A B` = a `B` anywhere under an `A`), and a predicate
  on an ancestor scopes the target (`Row[title="…"] Star`). There is **no `>`
  child combinator** — `A > B` is a parse error; use whitespace.
- If a query matches **zero** components it errors; if it matches **several** it
  errors and prints the candidates with their distinguishing properties — add a
  predicate to disambiguate, or append `#N`. ("Several match" is an ambiguity
  error listing candidates, **not** a "no such component" error — read it and
  disambiguate rather than assuming the component is unreachable.)
- Robust queries need a corpus with disambiguating **properties**; authoring a
  property like `FulfillmentTileButton.label` is what makes the element
  addressable (see the `sightmap-authoring` skill).
- `--sightmap-dir` (default `.sightmap`) controls which corpus resolves a query.

## Console & network — the runtime view of the corpus

The `browser start` daemon owns the session and runs a **collector** that buffers
console messages and network requests from every tab for the life of the session
(bounded ring buffers, ~1000 of each; successful scripts, styles, images and fonts
get a ring of their own so a dev server's hundreds of module scripts cannot evict the
app's `/api` calls). Read them with thin query commands — no
re-attaching to Chrome, and with history the transient page commands never saw.

These are not just raw debug streams. Each captured record is matched against the
corpus's `requests:` and `messages:` entries (see the `sightmap-authoring`
skill), so every line **leads with a `[MatchedName]` slot** — or `[--]` when
nothing in the corpus classified it — the way a snapshot foregrounds a node's
`[Component]`. Matched records also **trail their extracted `{name=value}`
properties**.

| Command | What it does |
|---------|-------------|
| `sightmap console list [--level error] [--tab ID] [--limit N]` | Console messages, each led by its `messages:` match (uncaught exceptions fold in as `level=exception`). |
| `sightmap console get <index>` | One console message + any extracted stack `properties:`. |
| `sightmap network list [--type XHR] [--url /api] [--filter REGEXP] [--static] [--tab ID] [--limit N]` | Requests: `[Match] method url → status (type) {prop=value}`. Successful static assets are hidden unless `--static`. |
| `sightmap network get <index> [--response-file F]` | One request + its `requests:` match, a `Properties:` block, and response body (save with `--response-file`). |

Reading the slot:
```
[AuraAction]         POST /aura → 200 (XHR) {first_action_state=SUCCESS}   # matched a requests: entry + extracted a body value
[AuraAction]         POST /aura → 200 (XHR) {first_action_state=ERROR}     # 200 OK, but the body says it failed — the payoff of request properties:
[--]                 GET  /favicon.ico → 200 (Image)                       # unmatched: no corpus entry, unstructured
[DeprecatedChartApi] warn  ...stackhbar.js has been deprecated             # matched a messages: entry
```
The `[--]` slot is deliberately quiet — it marks a record unclassified without
drowning the matched ones.

- These need a running `browser start` session — that's where the collector
  lives. Records from **before** the session started aren't captured, so in an
  attached/degraded session (or right after `start`) **reproduce the traffic**:
  refresh the page or re-trigger the action, then read. (A SPA may never reach
  `wait-for --load`; prefer `wait-for --url`/`--selector`, or just re-read after
  a short pause.)
- `network list` **hides successful static requests** (Script, Stylesheet, Image,
  Font, Media, Manifest with a 2xx or 304) by default, as `playwright-cli requests`
  does, so a Vite page's ~200 `/src/...` modules don't bury the app's calls. A failed
  asset (404, 5xx, refused) still shows. Pass `--static` to see scripts and styles
  too; `--type`, `--url` and `--filter` narrow either list further. `network get
  <index>` works for hidden requests as well.
- `--url` is a substring match, not a path anchor (`--url /aura` also catches
  `.../auraFW/...`) — narrow it when a route is noisy. `--filter` takes a regular
  expression (`--filter '/api/(issues|me)$'`), unanchored.
- To find failures: reproduce, then scan the `[...]` slots in `console list`
  (`--level error`) and `network list`; `network get <index>` pulls the body +
  properties.

## Gotchas

**`browser navigate` takes a positional URL, not a `--url` flag.**
```bash
sightmap browser navigate 'https://...'     # ✓ correct
sightmap browser navigate --url 'https://'  # ✗ wrong — passes literal "--url" as the URL
```

**`browser fill` replaces the value and checks that it did.**
`fill` focuses the field, selects its whole content through the DOM (input, textarea,
contenteditable; a Ctrl/Cmd+A chord with the `selectAll` command is the fallback for
anything else), then types, so the page sees real key events. It reads the value back and
**errors** when the field is still empty (a React-controlled input that dropped the keys) or
when the old value was **not replaced** (the field now holds old and new text, e.g.
`2026-04-102026-04-17`); the message names both. A masked or reformatted value is accepted.
On such an error retry with `--clear`, which empties the field through the element's native
value setter first (input, textarea, select, contenteditable) and then types. `--clear`
fails loudly when nothing editable is focused instead of typing on regardless.
```bash
sightmap browser fill --clear 'SearchInput' 'burrito'
```

**`browser fill` sets, not types, date/time/color/range inputs.**
Typing into `<input type=date>` goes through Chrome's segmented year/month/day editor and mangles the value. For `date`, `time`, `datetime-local`, `month`, `week`, `color` and `range`, `fill` sets the value whole (native setter, then `input` and `change`), including in iframes, and reads it back. Pass the format the control stores: `fill 214 2027-04-16`, not `04/16/2027`. A value the control can't parse errors rather than leaving the field empty.

**Input commands foreground a hidden tab.**
Chrome is reported to hit-test and focus only on the visible tab, so a click sent to a background one could print success and do nothing. `click`, `fill`, `hover`, `keypress`, `scroll` and `drag` now bring a hidden `--tab` to the front first and print `brought tab ID to front` on stderr; the tab stays in front.

**`fill` errors when its click did not focus the field** (`did not focus it`), instead of typing into nothing; `fill --clear` says `the field did not take focus` where it used to print a raw JS exception.

**`browser click` scrolls the target into view and refuses off-screen / covered clicks.**
`click` scrolls the element to the center of the viewport, then verifies its center is actually the top-most element there before dispatching. It **errors** (rather than silently no-op'ing) when the target can't be positioned in the viewport, or is `covered by another element` — the signal of an open overlay/modal or a target you need to reveal first. The confirmation line reports the real post-scroll coordinates it clicked.

**After an action that should navigate, wait for the destination — don't snapshot immediately.**
SPA (client-side) navigation lands *after* `click` returns, so a snapshot taken right away shows the old page. `click` deliberately does not guess or wait; make the wait an explicit step, the way Playwright/Selenium do:
```
sightmap browser click 'WorkItemRow[key="FALCON-7"]'
sightmap browser wait-for --view WorkItemDetail      # or --component 'WorkItemDetail', or --url '/browse/'
```
`wait-for` auto-retries until the postcondition holds or it times out (loudly). Record per-app async quirks (which actions navigate, slow routes) in the relevant view/component `memory` so the next agent knows to wait.

**`browser eval` cannot return DOM elements.**
Only JSON-serializable values are returned. `document.querySelector(...)` returns an error reference. Extract what you need instead: `document.querySelector("sel")?.textContent`.

**Stale session files.**
`browser status` probes the CDP endpoint (not just the `.session` file): it reports `✗ unreachable` and removes the stale session file when Chrome is gone. If a command still fails with a CDP error, run `browser start` again.

**Always pass `--tab` when several tabs are open.**
Page commands auto-pick the lone content tab but error (listing tabs) when zero or several are open. `browser start` prints your tab ID; thread it through as `--tab <ID>`.

**`browser navigate` prints the final URL.**
After a redirect — server-side *or* client-side (an SPA auth guard bouncing `/login → /`, or `/ → /workspace`) — `navigate` prints `(redirected to FINAL)` so you know where you actually landed, not just the URL you asked for.
