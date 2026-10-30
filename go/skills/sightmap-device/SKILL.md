---
name: sightmap-device
description: Use when driving a native iOS app in the local iOS Simulator, or an Android app on an emulator or device, with a sightmap — booting a simulator, installing and launching the app, taking annotated component snapshots of the accessibility tree, tapping and typing by accessibility identifier or component query, and reading the app's console and network requests. Activate when the user wants to run, try, test, or reproduce a flow in an iOS app on the Simulator, inspect an iOS screen, or author `.sightmap/` definitions for an iOS or React Native app.
activation:
  - .sightmap/ directory exists in an iOS or React Native project
  - driving an app in the iOS Simulator against a sightmap corpus
  - driving an Android app on an emulator or device against a sightmap corpus
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
sightmap device network list                       # the app's requests, matched against requests:
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
   `network list` / `network get N` (requests), `screenshot --out shot.png`
   (visual evidence).

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
off the screen, under the status bar (where a tap scrolls the list to the top
on iOS, and goes to the status bar on Android) or a navigation bar, outside the visible part of its list, under the tab bar, toolbar, navigation bar or keyboard, under a
React Native LogBox toast, under any control drawn over it (e.g. a custom tab
bar of plain buttons floating over a list), or within 24 pt of a floating
dev-menu button, and says what to do. When the center is covered but part of
the target shows (a sheet's dimmed backdrop), it gives a `tap-xy` point there. Long SwiftUI/UIKit lists expose only the rows near the screen,
so a row further down is "not found" until you scroll:
`scroll down --until TARGET` scrolls slowly enough not to fling past it, nudges
the target into the middle of the screen, stops once it is tappable there, and
refreshes the snapshot ids. A target matching several elements stops at the
first tappable one. `[text="…"]` is the whole label; use `[text*="…"]` for part
of it.

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
| `start [--device NAME\|UDID] [--app PATH \| --bundle-id ID] [--label L] [--network auto\|har\|rn-cdp\|summary\|off] [--detach]` | Boots, enables app accessibility, installs, launches, runs the daemon and begins a run log. |
| `status` / `stop [--shutdown]` | Session health / stop the daemon and the app (`--shutdown`: the simulator too). |
| `launch` / `relaunch` | Foreground the app / restart it (re-enables accessibility first). |
| `snapshot [--view V] [--include-offscreen] [--out F] [--tree-out F]` | Annotated tree. `--out F` writes the text plus `F.tree.json`, which `sightmap explain --snap` and `sightmap coverage` read. |
| `click TARGET [--settle DUR]` | Re-resolves, checks it's on screen, uncovered and enabled, taps its center. |
| `fill TARGET TEXT [--clear] [--settle DUR]` | Taps, types (appends unless `--clear`), errors if the field doesn't show the text. `fill TARGET "" --clear` empties it; a field then showing only its placeholder reports value `""`. |
| `scroll up\|down\|left\|right [--in TARGET] [--until TARGET] [--max N]` | One page, or until a target is tappable (default `--max 20`). `--in` scrolls inside that element. |
| `swipe X1 Y1 X2 Y2 [--duration S]`, `tap-xy X Y`, `type TEXT`, `key NAME`, `button NAME`, `openurl URL` | Raw input in screen points. Keys: `return delete tab escape space` and arrows. Buttons: `home lock siri side apple_pay volume_up volume_down`. |
| `screenshot --out F.png` | PNG of the screen. |
| `console list [--limit N] [--grep S] [--level L[,L]] [--all] [--since INDEX] [--follow]` / `console get N` | The app's os_log/Logger output and its stdout/stderr. Levels: `debug info default error fault stdout stderr`. `--grep` is case-insensitive and covers every source; `--all` adds Apple framework errors, hidden by default as noise. `--since` pages forward (pass back `last`); `--follow` streams. The session keeps the latest 5,000 entries (`start --console-limit N`); a list says when earlier ones were dropped. |
| `network list [--limit N] [--grep S] [--source har\|rn-cdp\|summary] [--since INDEX] [--follow] [--unredacted]` / `network get N [--unredacted] [--request-file F] [--response-file F]` | The app's requests (below). |
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
  when it has a `testID`, or a label or `testID` plus a tappable trait, and it
  is enabled. A Pressable without `onPress` (a DataTable cell, a decorative
  IconButton) reads as disabled and is left out; a disabled control with a
  label or `testID` still counts.
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

## Network

`network list` shows the app's requests, oldest first, each led by the
`requests:` definitions that match it and trailed by the properties they
extract, like `sightmap browser network list`:

```
#0 [CreateOrder]          POST http://127.0.0.1:18765/anything/sightmap/order-1 → 201 (56 ms, har) {order=1, echoed=1}
```

`network get N` adds headers and bodies; `--grep`, `--source`, `--since`
and `--follow` work as in `console list`. Each entry's `source` is where it
came from. `start --network auto` (default) picks, in order:

| Source | Gives | Needs |
|---|---|---|
| `har` | Full URL with query, method, status, timing, all headers, bodies ≤ 1 MiB; all `URLSession` traffic, React Native `fetch` included | The app signed with the **`get-task-allow`** entitlement |
| `rn-cdp` | JS `fetch`/XHR of a React Native Debug build via Metro: URL, method, JS-set headers, bodies | Metro running (`--metro-port N`, else the app's `RCT_jsLocation`, else 8081) |
| `summary` | Status, timing, bytes; method and host (no path) when HAR is off. Never matches a route | Nothing |

- **`get-task-allow`**: CFNetwork logs HAR only for a process with it. A
  Debug build Xcode signs for development has it; an ad hoc simulator build
  (`CODE_SIGN_IDENTITY = "-"`) needs `CODE_SIGN_ENTITLEMENTS` pointing at a
  plist with `<key>get-task-allow</key><true/>`. Without it, `status` says
  `har unavailable: … lacks the get-task-allow entitlement` and the session
  falls back. Release builds never have it.
- `status` (and `doctor`, while a session runs) shows each source's state
  and why the others are not used. HAR misses requests already under way
  when capture began; `summary` lists those, host-free.
- HAR capture uses private CFNetwork switches inside the simulator; `stop`
  turns them off (a dead session's capture lapses within 10 minutes).
- **Redaction**: `Authorization`, `Cookie`, `Set-Cookie`,
  `Proxy-Authorization`, and any header, query parameter, form field or JSON
  key containing `token`, `password`, `secret` or `key` show as `<redacted>`,
  in properties and in `--request-file` / `--response-file` too.
  `--unredacted` shows them; ask before using it.
  Entries stay in the daemon's memory (latest 1,000, bodies capped at
  256 KiB) and never reach the run log.
- To author a `requests:` entry, copy the path from `network list`, write a
  `route` glob (`/api/orders/*`) and `method`, add `properties:` with
  `extract: { from: req.body|rsp.body|req.headers|rsp.headers, path: … }`,
  and check the match on the next `network list` (the corpus reloads).

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

## Android

The same commands drive an Android emulator or device through `adb` (any OS,
only the SDK platform-tools needed). `sightmap device doctor --platform android`
checks for adb and a device or AVD. Start with an `.apk` or `--package`, which
imply `--platform android`:

```bash
sightmap device start --app app/build/outputs/apk/debug/app-debug.apk --detach
sightmap device start --package com.example.app --device Pixel_8_API_35 --detach
```

What differs from iOS:

- `--device` is an `adb devices` serial, a model, or an AVD name (a stopped AVD
  is booted).
- Each read is a `uiautomator dump`, a second or more, but it waits for an idle
  screen, so `click` taps after one read instead of waiting for two to agree. If
  it fails with "could not get idle state", turn off animations
  (`adb shell settings put global animator_duration_scale 0`, and the
  transition and window scales). It can't run beside an Appium UiAutomator2
  server.
- Points are dp: divide screenshot pixels by dpi/160.
- A sleeping display reads as no window at all. `start`, `relaunch` and a read
  that finds no window wake it (an emulator is also set to stay on); if a
  snapshot says the display "was asleep", unlock the device by hand (a PIN or
  pattern lock can't be dismissed over adb).
- `identifier` / `testid` is the resource-id without its package
  (`login_submit`, not `com.example:id/login_submit`). Compose `testTag`s show up
  only with `Modifier.semantics { testTagsAsResourceId = true }` on an ancestor;
  without it, select Compose elements by role and `text`.
- Roles come from widget classes (`Button` → `button`, `TextView` → `text`,
  `EditText` → `textfield`/`securetextfield`, scrollable views → `scrollview`,
  `RecyclerView` → `collection`, the rest → `other`). A clickable `other`
  without text, such as a list row, is named by its children's text.
- A field's label is its hint; its value is its text.
- Icon-font glyphs (private-use characters from react-native-vector-icons or
  Material Symbols) are dropped from names: a tab is `Home`, not
  `\U000f0538, Home`.
- Permission dialogs and other apps' windows appear as `[Overlay]`.
- What is in front follows elevation and `zIndex` (uiautomator's
  `drawing-order`), not tree order: a row under a header that floats over the
  list is refused as covered. A button the app draws under the status bar,
  outside a list, can't be scrolled clear; use `button back`.
- When the home screen is in front, the snapshot says `[Not in front]` and
  actions fail with `not_in_front`: run `launch`. `button back` on the app's
  first screen closes the app, an easy way to lose it mid-exploration.
- The keyboard is the part of the input method window that is drawn and takes
  touches, not its frame (which spans the screen on recent Android). Gboard on
  an emulator with a hardware keyboard shows only a small floating toolbar;
  it reads as its own `keyboard` node and covers only what's under it.
- `button back` goes back or hides the keyboard; `button` also takes `home`,
  `menu`, `app_switch`, `lock`, `volume_up`, `volume_down`.
- `type` / `fill` are ASCII only (`adb shell input text`).
- `network` is iOS-only for now; on Android it fails with `unsupported`.
- `console` is the app's logcat (tag as subsystem; `System.out` reads as
  `stdout`).
- If the app never reaches its first screen (the snapshot says `[Not in front]`),
  it crashed: read `adb logcat -d -b crash`. A release build missing env that
  its JS bundle reads at build time crashes like this.

### Setting up Android from nothing (macOS)

When `doctor --platform android` finds no adb or no AVD and nothing is
installed, this is the shortest working setup (no Android Studio):

```bash
brew install openjdk@17 && brew install --cask android-commandlinetools
export JAVA_HOME=/opt/homebrew/opt/openjdk@17/libexec/openjdk.jdk/Contents/Home
export ANDROID_HOME=$HOME/Library/Android/sdk
SDKM=/opt/homebrew/share/android-commandlinetools/cmdline-tools/latest/bin/sdkmanager
yes | $SDKM --sdk_root=$ANDROID_HOME --licenses
$SDKM --sdk_root=$ANDROID_HOME --install "cmdline-tools;latest" platform-tools emulator \
  "platforms;android-35" "build-tools;35.0.0" "system-images;android-35;google_apis;arm64-v8a"
export PATH="$ANDROID_HOME/platform-tools:$ANDROID_HOME/emulator:$ANDROID_HOME/cmdline-tools/latest/bin:$ANDROID_HOME/build-tools/35.0.0:$PATH"
avdmanager create avd -n Pixel_8_API_35 -k "system-images;android-35;google_apis;arm64-v8a" -d pixel_8
```

Put the exports in the user's shell profile. `build-tools` on PATH gives
`aapt2`, which `start --app` uses to read the package name. Then
`sightmap device start --app X.apk --device Pixel_8_API_35 --detach` boots the
AVD (about 80 s cold, under 20 s from its snapshot later). Building the app
itself needs whatever its Gradle build needs (usually JDK 17 and an SDK
platform it downloads on first build).

- If the emulator exits during boot, the error quotes its log
  (`$TMPDIR/sightmap-emulator-<AVD>.log`).
- `QT_QPA_PLATFORM` set to anything but `cocoa` makes the macOS emulator exit;
  `start` drops it when it boots an AVD, but an emulator you launch yourself
  needs it unset.
- Don't change the AVD's `hw.keyboard`: with it off, the person watching can't
  type into the emulator from their own keyboard, and `fill` doesn't need it.

## Known limits

- iOS: Simulator only, Apple Silicon only. No physical iOS devices.
- No hover or file upload; for another screen size, pick a different `--device`.
- The helper uses Apple private frameworks (via FBSimulatorControl); a new
  Xcode release may need a sightmap update. `doctor` says so when the helper
  fails to build.
- On iOS 26 a SwiftUI tab bar can show up as one `other "Tab Bar"` element
  without its tabs; switch tabs with `tap-xy` from a screenshot.
- The corpus is always the session's `--sightmap-dir`.
- Network capture sees `URLSession`/CFNetwork traffic (plus, via Metro, a
  React Native Debug build's JS traffic); raw sockets and custom stacks are
  invisible.
- Scrolling to a far row takes about 3 s per screenful (more for a React Native
  `FlatList`, whose tree is larger).
