---
"@sightmap/sightmap": patch
---

The overlay extension picks views the way the CLI does. Its route matcher treated `:param` segments as literal text, so a view routed `/ui/:org/settings` never activated and hover fell back to globals; it now ports the Go matcher (`:param`, in-segment `*`, `**` as zero or more segments, trailing-slash normalization). Only the most specific matching view is active, ties going to the first declared, instead of every matching view's components at once. The extension version is bumped so existing installs pick up this and the earlier resolver fixes.
