---
"@sightmap/sightmap": patch
---

Fix `browser mcp call` failing against native Chrome WebMCP. `executeTool` on a native `document.modelContext` requires the arguments as a JSON string (and returns the result as a JSON string), whereas a JS polyfill takes and returns objects. `mcp call` passed arguments as an object unconditionally, so it worked against a polyfill but failed on native with "Failed to parse input arguments". It now detects the native surface (the built-in `executeTool` reads as `[native code]`) and serializes the arguments accordingly; the native JSON-string result was already normalized.
