---
"@sightmap/sightmap": minor
---

WebMCP support: turn a mapped site into agent-callable tools.

- **New `sightmap webmcp` command.** `webmcp init` scaffolds a draft `webmcp.tools.yaml` manifest from the corpus (one read tool per view, one api stub per request), `webmcp validate` compile-checks every component / property / view / request reference the manifest names, and `webmcp generate` emits the snippet, ES-module, and userscript bundles that register the tools on `document.modelContext` (`--check` compares against files you already generated).
- **New `sightmap-webmcp` skill.** The authoring loop that produces the manifest: walking user-goal trajectories with the browser CLI (component queries, network traces), choosing each tool's kind (api / fetched page read / live flow), folding corpus `memory:` hazards into the tool descriptions agents decide by, and live-verifying every generated tool before publishing. Ships in the plugin, the embedded skills (`sightmap skills install`), and this package.
