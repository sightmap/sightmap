---
"@sightmap/sightmap": patch
---

`browser mcp` no longer points at Chrome flags that do not exist. The "no WebMCP
tools here" error used to suggest `--enable-blink-features=ModelContext,ModelContextTesting`
and `--enable-features=DevToolsWebMCPSupport`; Chrome silently ignores those
names, so following them left `document.modelContext` undefined on Google Chrome
and made the native path look unshipped. The message and the `browser mcp` docs
now name the real switch, `--enable-features=WebMCPTesting` (the same feature as
`chrome://flags/#enable-webmcp-testing`). Fixes #413.
