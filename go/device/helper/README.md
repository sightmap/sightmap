# sightmap-device-helper

A long-lived Swift process that reads the iOS Simulator's accessibility tree and
sends touch, keyboard and button input for `sightmap device`. It is built on
Meta's FBSimulatorControl, consumed as idb v1.7.7's prebuilt xcframeworks, whose
URLs and checksums are pinned in `Package.swift`.

## How users get it

These sources are embedded in the `sightmap` binary (`go/device/helper.go`) and
built with the local Xcode the first time it is needed (`sightmap device doctor`
or `start`), into `~/Library/Caches/sightmap/device/helper/<source hash>/`. A
build takes 10–20 s plus a ~10 MB framework download the first time; a changed
helper rebuilds under a new hash. The build passes `--disable-keychain
--disable-netrc` so SwiftPM never offers a saved credential (a keychain prompt)
while downloading public files, and keeps its caches under the cache root.

Building on the user's machine was chosen over shipping a prebuilt binary:

- the helper links Apple private frameworks (CoreSimulator,
  AccessibilityPlatformTranslation) that track the installed Xcode, so a build
  against the user's own Xcode is the one most likely to load;
- `go install`, the npm package and release archives all carry the same
  sources, with no per-platform signing and notarization pipeline;
- Xcode is required anyway, for the simulator itself.

`SIGHTMAP_DEVICE_HELPER=/path/to/sightmap-device-helper` uses a prebuilt
binary instead; `SIGHTMAP_DEVICE_CACHE` moves the cache.

Requirements: an Apple Silicon Mac (the frameworks are arm64 only) on macOS 15+,
full Xcode 26+ selected with `xcode-select`, opened once.

## Protocol

JSON lines on stdin/stdout, one request per line:

```json
{"id": 1, "cmd": "describe", "args": {"udid": "…"}}
{"id": 1, "ok": true, "result": {"screen": {"w": 402, "h": 874}, "root": {…}}}
{"id": 2, "ok": false, "error": {"code": "not_booted", "message": "…"}}
```

| cmd | args | result |
|---|---|---|
| `hello` | — | `{version, xcode, fbsimctl}` |
| `describe` | `udid` | `{screen: {w, h}, root: Node, overlay?}`; `Node = {role, subrole, type, label, identifier, value, frame: {x, y, w, h}, enabled, traits, children}` in points, top-left origin. `overlay` is true when the tree is a system sheet from another process. |
| `tap` | `udid, x, y` | `{}` |
| `type` | `udid, text` (printable ASCII, newline, tab) | `{}` |
| `swipe` | `udid, x1, y1, x2, y2, duration?` | `{}` |
| `button` | `udid, name`: `home lock siri side apple_pay volume_up volume_down` | `{}` |
| `key` | `udid, key`: a name (`return delete tab escape space` arrows `home end page_up page_down forward_delete`), one character, or a HID usage | `{}` |

Error codes: `bad_request`, `unknown_cmd`, `device_not_found`, `not_booted`,
`ax_disabled` (no elements), `ax_unavailable`, `ax_error`, `hid_error`,
`unsupported_text`, `internal`. Protocol output goes to a private duplicate of
stdout; diagnostics go to stderr (`SIGHTMAP_DEVICE_HELPER_DEBUG=1` for verbose
framework logs).
