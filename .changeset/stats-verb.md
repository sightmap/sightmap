---
"@sightmap/sightmap": minor
---

Add corpus statistics, in the library and on the CLI.

New public API: `sightmap.Stats`, `sightmap.ViewStats`, and `(*Corpus).Stats()`, so any consumer of a loaded corpus — the atlas index generator, Subtext — gets the counts without shelling out to the CLI. `Components` counts distinct component names corpus-wide (a global reused by three views is one component), while `Properties` and `Memory` are summed over distinct component *definitions*, so a `$ref`-expanded copy counts once but two views that each define a different component under the same local name both count. `Stats.IsEmpty` reports a corpus with nothing in it — memory alone is not nothing.

New `sightmap stats` verb over that API: the totals plus a per-view component/request table, and `--json` for a stable machine-readable form (`views` / `components` / `requests` / `properties` / `memory` / `per_view`) suitable for CI consumers. `stats` refuses a corpus that `sightmap validate` rejects, since the loader drops the definitions it cannot resolve and the counts would silently under-report; in `--json` mode the failure is itself JSON (an `error` key plus `diagnostics`), so a consumer always has something to parse.
