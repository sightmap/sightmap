---
"@sightmap/sightmap": minor
---

Add `sightmap atlas`, three verbs over the community atlas (github.com/sightmap/atlas): `find` searches it, `list` browses it, `add` installs one corpus into `.sightmap/`. They are grouped under `atlas` because a bare `sightmap add` would be ambiguous in a tool for authoring corpora, where adding a view or a component is the other thing it could mean.

`atlas find` matches slug, name, domains, categories, and description, ranked so an exact domain match comes first, because an agent about to automate a site starts from a URL and has no way to guess a slug. Domains are normalized, so `https://squareup.com/checkout`, `www.squareup.com`, and `squareup.com` all land on the same entry. Each hit prints its own `sightmap atlas add` command, so a search hands off to an install with nothing to assemble. A query that matches nothing exits 0: asking whether the atlas has a site is a question, and "no" is an answer. `--category`, `--limit`, and `--json` narrow and reshape the results; `atlas list` is the same search with an empty query.

`atlas add` fetches one `.tar.gz` and never reads the index, so an index outage or a schema change cannot stop an install and the atlas can grow index fields without waiting for a CLI release. It refuses a non-empty target before touching the network, extracts into a temporary directory beside the target, loads the staged corpus, and renames it into place. The rename is what makes it atomic. There is no `--force`. Deleting a directory that holds your work is your call.

The atlas wire contract ships as a public package, `github.com/sightmap/sightmap/go/atlas`, so the atlas publisher CI enforces the same rules the CLI does instead of re-implementing them: the index schema and its version gate, the search ranking, the 24-hour index cache at `~/.sightmap/atlas/index.json`, the fail-closed validators, and the `Install` operation.

Everything the atlas serves is untrusted. Fetches are HTTPS-only (loopback excepted) with the policy re-applied on every redirect hop, so a `302` cannot downgrade an install to plaintext. An archive is capped on the wire and again decompressed, per file, and by member count; every member must be a regular file or directory under `.sightmap/`, with no absolute path, traversal, symlink, or control character. Names, descriptions, domains, and categories are escaped before they reach a terminal, and an entry whose slug could not be installed is never offered with an install command.
