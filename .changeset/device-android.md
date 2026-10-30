---
"@sightmap/sightmap": minor
---

`sightmap device` drives Android apps too. `start --platform android` (implied by an `.apk` or `--package`) works with a connected emulator or device through `adb` and boots a named AVD. Snapshots map `uiautomator dump` onto the same tree as iOS, in dp, with resource-ids (and Compose test tags published as resource-ids) as identifiers. Taps, swipes, typing and keys go through `adb shell input`, `console` reads the app's logcat, and `doctor --platform android` checks for adb and a device. Nothing is installed on the device besides the app.
