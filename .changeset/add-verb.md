---
"@sightmap/sightmap": minor
---

Add `sightmap atlas`: `find` searches the community atlas at sightmap.org/atlas, `list` browses it, and `add` installs one corpus into `.sightmap/`.

`find` matches slug, name, domains, categories, and description, ranked so an exact domain match comes first, because an agent about to automate a site starts from a URL and has no way to guess a slug. Each hit prints its own `sightmap atlas add` command, and a query that matches nothing exits 0.

`add` fetches one `.tar.gz` and never reads the index, so an index outage cannot stop an install. It refuses a non-empty target before touching the network; there is no `--force`. `--index` and `--source` point the verbs at a mirror or a private corpus store, under the same HTTPS-only transport policy and the same archive caps.

New public API: `sightmap.Totals`, the five corpus-wide counts split out of `sightmap.Stats`, which now embeds it. Field access and the `sightmap stats --json` output are unchanged — the JSON is byte-identical — but a composite literal must now write `Stats{Totals: Totals{Views: 1}}`. The split exists because a published catalog entry carries these five numbers as its `stats` object and puts the per-view rows in a sibling field, so `atlas.Entry.Stats` is a `sightmap.Totals`: one definition of what the counts mean, and no `PerView` field that no valid catalog could ever fill. As a side effect `sightmap atlas find --json` now prints a count that is zero rather than omitting it, and reports `properties` and `memory`, which it previously discarded.
