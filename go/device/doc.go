// Package device drives a native iOS app in the local iOS Simulator, the way
// package browser drives Chrome: it reads the app's accessibility tree, layers
// a .sightmap/ corpus onto it, acts on elements by id, identifier or component
// query, and collects the app's console. `sightmap device` is its CLI.
//
// The pieces:
//
//   - Normalize turns the helper's describe result into a sightmap
//     ComponentNode tree, so the corpus matcher, coverage and the offline tools
//     read a screen like a web page.
//   - Annotate applies a corpus to a Tree. A native screen has no URL, so the
//     view is named by the caller or inferred from the tree (InferView) until
//     SEP-0024 gives views an identity of their own.
//   - Session drives one app: snapshot, click, fill, scroll and raw input, each
//     recorded in a run log. Serve runs a Session behind a Unix socket and a
//     session file in the sightmap directory; Call talks to it.
//   - The helper (helper/) is a Swift program on FBSimulatorControl that reads
//     the tree and sends input over JSON lines. Its sources are embedded and
//     built with the local Xcode on first use (BuildHelper).
//
// Simulator lifecycle, installs, launches, screenshots and logs go through
// `xcrun simctl`.
package device
