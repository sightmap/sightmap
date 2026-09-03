---
"@sightmap/sightmap": minor
---

Add `browser mcp list` and `browser mcp call` for the [WebMCP](https://webmachinelearning.github.io/webmcp/) tools a page exposes via `document.modelContext`. `mcp list` enumerates the tools (name, description, input schema; `--json` for full schemas) and distinguishes native, polyfilled, and absent WebMCP — failing loudly with the Chrome flags that enable native WebMCP when there is none. `mcp call <tool> --args '{…}'` resolves a tool by name and invokes it via `executeTool`, printing the structured result (surfacing any guidance the tool returns) and reporting not-found or thrown errors distinctly. One tool runs at a single point in time; there is no cross-navigation tool response.
