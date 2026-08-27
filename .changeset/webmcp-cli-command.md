---
"@sightmap/sightmap": minor
---

New `sightmap webmcp` command: generate WebMCP tool bundles (`document.modelContext`) from a corpus plus a `webmcp.tools.yaml` manifest. `webmcp init` scaffolds a draft manifest from the corpus (one read tool per view, one api stub per request), `webmcp validate` compile-checks every component/property/view/request reference, and `webmcp generate` emits the snippet / ES-module / userscript bundles (`--check` gates drift in CI). Byte-identical to the reference Node generator at `webmcp/` in the sightmap repo; the authoring loop is the new `sightmap-webmcp` skill.
