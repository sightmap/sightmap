---
"@sightmap/sightmap": patch
---

The `capture-baseline` selector profile now accepts the SVG adjusted element and attribute names (`clipPath`, `linearGradient`, `foreignObject`, `[viewBox]`, `[preserveAspectRatio]` and the rest of the HTML spec's closed list), matched exactly as written. Any other mixed-case name is still rejected. An unquoted attribute value that starts with a digit (`[tabindex=0]`) is still rejected, and the message now advises quoting it (`[tabindex="0"]`). The classification cases are shared as a conformance fixture at `spec/conformance/036-selector-classification.fixture/cases.json`.
