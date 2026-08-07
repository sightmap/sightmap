---
"@sightmap/sightmap": minor
---

Add `sightmap atlas`: `find` searches the community atlas at sightmap.org/atlas, `list` browses it, and `add` installs one corpus into `.sightmap/`.

`find` matches slug, name, domains, categories, and description, ranked so an exact domain match comes first, because an agent about to automate a site starts from a URL and has no way to guess a slug. Each hit prints its own `sightmap atlas add` command, and a query that matches nothing exits 0.

`add` fetches one `.tar.gz` and never reads the index, so an index outage cannot stop an install. It refuses a non-empty target before touching the network; there is no `--force`. `--index` and `--source` point the verbs at a mirror or a private corpus store, under the same HTTPS-only transport policy and the same archive caps.
