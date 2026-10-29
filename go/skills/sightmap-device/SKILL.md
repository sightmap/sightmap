---
name: sightmap-device
description: Use when driving a native iOS app in the local iOS Simulator with a sightmap — booting a simulator, installing and launching the app, taking annotated component snapshots of the accessibility tree, tapping and typing by accessibility identifier or component query, and reading the app's console. Activate when the user wants to run, try, test, or reproduce a flow in an iOS app on the Simulator, inspect an iOS screen, or author `.sightmap/` definitions for an iOS or React Native app.
activation:
  - .sightmap/ directory exists in an iOS or React Native project
  - driving an app in the iOS Simulator against a sightmap corpus
---

# Sightmap device: drive an iOS app in the Simulator

`sightmap device` drives an app in the iOS Simulator on this Mac the way
`sightmap browser` drives Chrome: it reads the app's **accessibility tree**,
layers your `.sightmap/` corpus onto it, and lets you **act on elements by
accessibility identifier or component query**. Matching uses sightmap's own
matcher, so `sightmap explain` and `sightmap coverage` agree with what a device
snapshot shows. Simulator only; nothing leaves the Mac except the app's own
traffic. To build the corpus itself, see the `sightmap-authoring` skill.

## Prerequisites

Apple Silicon Mac, macOS 15+, Xcode 26+ selected with `xcode-select` (not just
the Command Line Tools), and an iOS Simulator runtime. Install the CLI if
`sightmap version` fails (`npm install -g @sightmap/sightmap`).

Run **`sightmap device doctor`** first. It prints the exact fix for anything
missing (`--fix` runs the unprivileged ones) and builds the simulator helper on
first use: a small Swift program embedded in the binary, built with your Xcode
(a minute or two, then cached in `~/Library/Caches/sightmap/device`; override
with `SIGHTMAP_DEVICE_CACHE`). `SIGHTMAP_DEVICE_HELPER=/path/to/helper` uses a
prebuilt one instead.

Run every command from the project root: `--sightmap-dir` defaults to
`.sightmap` (there is no upward search). The session lives in
`.sightmap/.device-session`; gitignore it along with `.device-runs/`.

## The loop

```bash
sightmap device start --app path/to/MyApp.app --detach   # or --bundle-id for an installed app; --device "iPhone 17 Pro" or a UDID
sightmap device snapshot                           # annotated tree; each element gets an id for this snapshot
sightmap device click login-submit                 # identifier, snapshot id, component query, or selector
sightmap device fill login-email "a@example.com"
sightmap device snapshot                           # re-snapshot after every action that changes the screen
sightmap device console list --grep error          # the app's logs (print / os_log), live
sightmap device stop                               # --shutdown also shuts the simulator down
```

1. **Start** once, always with `--detach` (without it, `start` holds the shell
   as a foreground daemon). `status` reports health. `start` on a running
   session reinstalls (with `--app`) and relaunches the **same** app; it refuses
   a different app or device: `stop` first. The first `start` on a simulator
   that has never had app accessibility enabled reboots it once.
2. **Snapshot** before acting. Ids are per-snapshot, so act on the latest one.
   Prefer identifiers or component queries over ids in anything you write down.
3. **Act** with `click` / `fill`. Both re-read the screen just before tapping,
   so an identifier or query never goes stale. Lower level: `scroll`, `swipe`,
   `tap-xy X Y`, `type`, `key return`, `button home`, `openurl URL`. `tap-xy`
   and `swipe` take screen **points**, not pixels (a 390×844 pt screen is
   1170×2532 px in a screenshot); points off the screen are refused.
4. **Check** with `snapshot` (state), `console list` / `console get N` (logs),
   `screenshot --out shot.png` (visual evidence).

Every command takes `--json`. Errors are one line plus a `Next:` step. A flag
value that starts with `-` goes after `=`: `--grep=-foo`.

## Targets

`click`, `fill`, `scroll --in` and `scroll --until` take one target:

| Target | Example | Resolves to |
|---|---|---|
| Snapshot id | `12` | element 12 of the latest snapshot |
| Identifier | `login-submit` | the one element whose accessibility identifier is exactly this |
| Component query | `SignInButton`, `ProductRow[title^="Product 15"]`, `LoginForm SignInButton`, `ProductRow#2` | a component, filtered on extracted properties (`=` `^=` `*=`); whitespace is a descendant combinator; `#N` is a 0-based occurrence |
| Selector | `button[identifier="login-submit"]`, `[text="Not Now"]`, `cell#0` | a CSS selector over the normalized tree, for elements no component names; optional trailing `#N` |

A word without brackets or spaces is tried as an identifier first; otherwise a
target starting with a capital letter is a component query, and anything else
a selector. A target matching several elements errors and
lists them: add a predicate or append `#N`.

**Off-screen and covered elements.** `click` refuses a target whose center is
off the screen, under the tab bar, toolbar, navigation bar or keyboard, under a
React Native LogBox toast, under any control drawn over it (e.g. a custom tab
bar of plain buttons floating over a list), or within 24 pt of a floating
dev-menu button, and says what to do. Long SwiftUI/UIKit lists expose only the rows near the screen,
so a row further down is "not found" until you scroll:
`scroll down --until TARGET` scrolls slowly enough not to fling past it, stops
once the target is tappable, and refreshes the snapshot ids.

**Waiting.** Actions return as soon as the tap lands; navigation keeps
animating for a moment. If a snapshot still shows the old screen, snapshot
again. `click` and `fill` wait out short animations themselves: while the target
is missing, or the screen moves between two reads ~150 ms apart (a sheet
closing, a back navigation), they re-read for up to 3 s (`--settle 5s` to
wait longer, e.g. for a Debug React Native build redrawing after a back
navigation). They tap only once two reads in a row agree on where the target
and the screen are, so a tap never lands mid-transition; a screen that keeps
moving for the whole wait is refused as `unstable` rather than tapped. `--json`
reports how many re-reads it took as `retries`.

## Commands

| Command | What it does |
|---|---|
| `doctor [--fix]` | Checks macOS, Apple Silicon, Xcode, simulator runtimes and devices, Swift; builds the helper. |
| `start [--device NAME\|UDID] [--app PATH \| --bundle-id ID] [--label L] [--detach]` | Boots, enables app accessibility, installs, launches, runs the daemon and begins a run log. |
| `status` / `stop [--shutdown]` | Session health / stop the daemon and the app (`--shutdown`: the simulator too). |
| `launch` / `relaunch` | Foreground the app / restart it (re-enables accessibility first). |
| `snapshot [--view V] [--include-offscreen] [--out F] [--tree-out F]` | Annotated tree. `--out F` writes the text plus `F.tree.json`, which `sightmap explain --snap` and `sightmap coverage` read. |
| `click TARGET [--settle DUR]` | Re-resolves, checks it's on screen, uncovered and enabled, taps its center. |
| `fill TARGET TEXT [--clear] [--settle DUR]` | Taps, types (appends unless `--clear`), errors if the field doesn't show the text. `fill TARGET "" --clear` empties it; a field then showing only its placeholder reports value `""`. |
| `scroll up\|down\|left\|right [--in TARGET] [--until TARGET] [--max N]` | One page, or until a target is tappable (default `--max 20`). `--in` scrolls inside that element. |
| `swipe X1 Y1 X2 Y2 [--duration S]`, `tap-xy X Y`, `type TEXT`, `key NAME`, `button NAME`, `openurl URL` | Raw input in screen points. Keys: `return delete tab escape space` and arrows. Buttons: `home lock siri side apple_pay volume_up volume_down`. |
| `screenshot --out F.png` | PNG of the screen. |
| `console list [--limit N] [--grep S] [--level L[,L]] [--all] [--since INDEX] [--follow]` / `console get N` | The app's os_log/Logger output and its stdout/stderr. Levels: `debug info default error fault stdout stderr`. `--grep` is case-insensitive and covers every source; `--all` adds Apple framework errors, hidden by default as noise. `--since` pages forward (pass back `last`); `--follow` streams. The session keeps the latest 5,000 entries (`start --console-limit N`); a list says when earlier ones were dropped. |
| `run new [--label L]` / `run current` / `run note TEXT [--kind K]` / `run tail [--follow] [--since SEQ] [--run ID]` | The run log (below). |

## Reading a snapshot

```
[View: Login] (inferred)

[Guide]
EmailField     1
PasswordField  1
SignInButton   1

[Memory]
- Any email containing "@" and any non-empty password signs in.

--- component tree ---

1 application "Sightmap Sim Sample"
  2 text[identifier="login-title"] "Sightmap Sim Sample"
  3 [EmailField]
  4 [PasswordField]
  5 [SignInButton "Sign In"]

[Coverage] (visible only)
3 interactive · 3 direct T1 (100%) · 0 scoped T2 (0%) · 0 orphaned T3 ✓
```

Matched elements show `[Component prop="value"]`; unmatched ones show
`role[identifier="…"] "label" value="…"`. Markers: `(disabled)`,
`(partly off-screen)`, and with `--include-offscreen`, `(off-screen)`. Extra
sections appear when they apply:

| Section | Meaning |
|---|---|
| `[Overlay]` | A system sheet from another process (e.g. iOS's "Save Password?") is in front of the app; the tree is that sheet. `click` and `scroll` report targets as `covered` until you dismiss it: `click '[text="Not Now"]'`. |
| `[Mostly empty]` | The app exposes nothing (or only a small floating element); see *Empty tree*. |
| `[LogBox]` | A React Native LogBox toast covers the bottom; gives the `tap-xy` that hits its ✕. |
| `[Memory]` | `memory:` entries from the corpus for this view and its matched components. |
| `[Conflicts]` | Elements matched by several components; only one applied. |
| `[Off-screen]` | Count of hidden nodes; queries still resolve them. |

**Views.** sightmap requires a `route` on every view, and native screens have
no URL, so native corpora use short placeholder routes (`/login`,
`/products`). `sightmap device` ignores routes: the snapshot infers the view
whose own components match the most elements (`[View: Login] (inferred)`), or
pass `snapshot --view NAME`. SEP-0024 proposes a `screen:` selector to replace
the placeholders; it is not usable yet.

## Authoring `.sightmap/` for iOS

Use the `sightmap-authoring` workflow; only the selectors differ. Elements
expose `role` (from the accessibility type: `button`, `text`, `textfield`,
`securetextfield`, `switch`, `tab`, `cell`, `image`, `other`, …), and the
attributes `identifier` (the `accessibilityIdentifier`; `testid` is an alias),
`text` (the label) and `value`:

```yaml
version: 1
components:
  - name: TabBarItem
    selector: 'tab'
    properties:
      - name: title
        extract: { from: dom.text }
views:
  - name: Products
    route: /products          # placeholder; ignored by sightmap device
    components:
      - name: ProductRow
        selector: '[identifier^="product-row-"]'
        properties:
          - name: title
            extract: { from: dom.text }
          - name: number
            extract: { from: dom.attr, path: identifier, pattern: 'product-row-(\d+)' }
```

- **Write leaf selectors.** Only accessible elements reach the iOS tree, so an
  identifier on a non-accessible wrapper view never appears there:
  `[testid="login-form"] [testid="login-submit"]` matches nothing. Write
  `[identifier="login-submit"]`.
- Prefer `[identifier=…]`; fall back to `[text=…]` only for copy that won't
  change or be localized.
- The corpus reloads when a YAML file changes; snapshot again to confirm
  components resolve, and run `sightmap validate`.
- **SwiftUI gotcha:** `.accessibilityIdentifier` on a container overwrites the
  identifiers of its children. Put identifiers on leaf controls (buttons,
  fields, rows), not on the stack around them. If every child shows the same
  identifier, that's why. Tab bar items take the identifier set on the `Label`
  inside `.tabItem`.
- The system keyboard is not part of the app: its keys never match components
  and are left out of coverage.

## React Native apps

- **`testID` is the identifier.** Target it directly (`click login-submit`) and
  select on it (`[identifier="login-submit"]` or `[testid="login-submit"]`).
- **Put `testID`s on accessible elements**: a `Pressable`, a `TextInput`, a
  `View` with `accessible`. A `testID` on a plain wrapper `View` never reaches
  the tree.
- **`<Text testID>` doesn't reach the tree** on the New Architecture: the text
  shows up, its identifier doesn't. Wrap it in an `accessible` `View`, or select
  on `[text=…]`.
- **Role-less `Pressable`s are `other`**, not `button`; only
  `accessibilityRole="button"` makes a `button`. Write role-free selectors
  (`[identifier^="product-row-"]`). Coverage counts an `other` as interactive
  when it has a `testID`, or a label or `testID` plus a tappable trait.
- **React Navigation tabs** become `tab "Products"` (the "tab, 1 of 3" suffix is
  stripped). Rows behind its JS tab bar are refused as covered, like a native one.
- **Labels join child texts with ", "**: a row reads `"Product 1, $6.37"`.
- **`console.log`** arrives as `info` lines from subsystem
  `com.facebook.react.log` (`console list --grep TEXT`).

**Debug with Metro is fine** and cheaper than Release: start Metro
(`npx react-native start`, or `npx expo start --dev-client`) before `start`;
without it the app shows a red "No script URL provided" box. If edits don't
show, restart Metro with `--clear`. In a git worktree nested inside another
checkout, Watchman watches the parent: stop Metro, `watchman watch-del-all`, add
a `.watchmanconfig` containing `{}` at the worktree root (Watchman rejects an empty file), and restart Metro there. If
port 8081 is taken, run Metro with `--port N` and point the app at it:
`xcrun simctl spawn <udid> defaults write <bundle-id> RCT_jsLocation localhost:N`,
then `relaunch`. A Release build drops `console.log` below error unless the app
calls `RCTSetLogThreshold(.info)` natively.

**Dev-menu floating button.** An Expo dev client (and some RN debug setups)
draws a floating button in its own small window that catches taps beyond its
frame. `click` refuses targets within 24 pt of it and suggests a `tap-xy` on
another part of the target. Better: open the dev menu, Settings, turn off "Show
floating action button"; reopen the menu with Device → Shake (⌃⌘Z) in
Simulator. It is recognized heuristically (a dev-menu label, or a small square
element without an identifier drawn last at the root).

**LogBox.** In Debug, `console.error`/`console.warn` raise a toast over the
bottom of the screen. The snapshot's `[LogBox]` section gives the `tap-xy` for
its ✕ (it has no element of its own); tapping the toast itself opens the
inspector. `LogBox.ignoreAllLogs()` keeps toasts away.

## Empty or nearly empty tree

When `snapshot` shows `[Mostly empty]`, or `click` / `fill` / `scroll --until`
say the app exposes nothing, the causes in order are:

1. A mounted view hides the rest: `accessibilityViewIsModal = YES`, or
   `accessibilityElementsHidden` / `importantForAccessibility="no-hide-descendants"`
   on a root view. In RN this is often an invisible but mounted full-window
   overlay (a toast host, a modal backdrop). Unmount it, or set those props only
   while it shows.
2. App accessibility is off: `relaunch` turns it on (the error omits this when
   it's already on).
3. A system sheet is animating in: snapshot again in a moment.

## The run log

Every action appends one JSON line to `.sightmap/.device-runs/<run>.jsonl`:
`ts`, `run`, `seq`, `action`, `args`, the `target` (snapshot id, component,
identifier, text, role), the `view`, `ok`, and any `error`. `start` begins a
run; `run new --label L` starts another. `launch` and `relaunch` are logged as
such. `run note TEXT --kind K` records an annotation of your own (e.g. when a
recording you started begins or ends) among the steps. Another agent or tool can follow a
driving agent with `run tail --follow` (or poll with `--since SEQ`) and line up
its own data (network captures, analytics, logs) with the steps by timestamp.
`console list --grep` covers the app's own logs.

## Sample app

`go/device/testdata/SampleApp/build.sh` in the sightmap repo builds a small
SwiftUI app (login form, 200-row product list, tab bar) with `xcodebuild` and
prints the `.app` path; its `.sightmap/` is a worked example. Run the
end-to-end tests with `SIGHTMAP_DEVICE_E2E="iPhone 17 Pro" go test ./device -run TestE2E`
from `go/`.

Apps you build yourself must be built by Xcode (`xcodebuild` or the Xcode
app), not assembled by hand, so they carry the simulated entitlements. After
signing in, iOS may show a "Save Password?" sheet: it appears as `[Overlay]`.

## Known limits

- Simulator only, Apple Silicon only. No physical devices.
- No hover or file upload; for another screen size, pick a different `--device`.
- The helper uses Apple private frameworks (via FBSimulatorControl); a new
  Xcode release may need a sightmap update. `doctor` says so when the helper
  fails to build.
- On iOS 26 a SwiftUI tab bar can show up as one `other "Tab Bar"` element
  without its tabs; switch tabs with `tap-xy` from a screenshot.
- The corpus is always the session's `--sightmap-dir`.
- Scrolling to a far row takes about 3 s per screenful (more for a React Native
  `FlatList`, whose tree is larger).
