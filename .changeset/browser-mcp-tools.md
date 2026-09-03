---
"@sightmap/sightmap": minor
---

Add `browser mcp list` and `browser mcp call` for the [WebMCP](https://webmachinelearning.github.io/webmcp/) tools a page exposes via `document.modelContext`. `mcp list` enumerates the tools (name, description, input schema; `--json` for full schemas) and distinguishes native, polyfilled, and absent WebMCP — failing loudly with the Chrome flags that enable native WebMCP when there is none. `mcp call <tool>` resolves a tool by name and invokes it via `executeTool`; arguments are supplied as a JSON object (`--args`) and/or repeatable `--param key=value` pairs. It unwraps the standard `CallToolResult` envelope (rendering the tool's text/structured content and any guidance as itself rather than a stringified blob), exits non-zero when the tool reports `isError` so it is scriptable, and reports not-found or thrown errors distinctly. One tool runs at a single point in time; there is no cross-navigation tool response.
