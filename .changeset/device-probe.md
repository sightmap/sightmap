---
"@sightmap/sightmap": minor
---

`sightmap device start --probe` loads an optional in-app probe into the simulator app. Snapshots then carry a `view[type="…"]` node for each of the app's SwiftUI view types (stored fields as redacted attributes) and one `text` node per Text, with `field`, and for formatted Texts the unformatted `value` and a `format`. Corpora can read the fields of a row whose accessibility label joins them exactly, through child components. No app changes or `get-task-allow` are needed. `doctor` builds the probe and `status` reports its state. If the app doesn't load the probe, snapshots fall back to the plain tree with a note.
