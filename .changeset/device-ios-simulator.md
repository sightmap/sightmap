---
"@sightmap/sightmap": minor
---

Add `sightmap device`: drive a native iOS app in the local iOS Simulator the way `sightmap browser` drives Chrome. `snapshot` annotates the app's accessibility tree with your `.sightmap/` corpus using sightmap's own matcher, `click` / `fill` / `scroll` act on snapshot ids, accessibility identifiers, component queries or selectors, and `console` and the run log (`.sightmap/.device-runs/`) record what happened. The simulator helper is embedded as source and built with the local Xcode on first use; `sightmap device doctor` checks the Mac and builds it. Ships with the `sightmap-device` skill.
