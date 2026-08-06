---
"@sightmap/sightmap": minor
---

Add `sightmap add <slug>`: install a published sightmap corpus from the community atlas (github.com/sightmap/atlas) into the current project, so one command from a gallery page yields a working `.sightmap/` directory. Looks the slug up in the atlas index (`--index` overrides the URL for mirrors and tests), suggests the closest slugs on a miss, fetches the entry's files pinned to its commit when one is published, and writes them into `--target` (default `./.sightmap`).

The atlas wire contract ships as a new public package, `github.com/sightmap/sightmap/go/atlas`, so the atlas publisher CI can enforce the same rules the CLI enforces instead of re-implementing them: the index schema and its version gate, the `<root>/<ref>/entries/<slug>/<path>` URL layout (one rule for GitHub raw, GitHub Enterprise raw, and a plain mirror), the fail-closed validators for slugs, commits, and corpus paths, a fetch client that holds its HTTPS-only policy across redirects and caps response size, and an `Install` operation.

`add` is fail-closed and atomic. It checks local preconditions before any network I/O, fetches every file (concurrently) before writing any, stages the result and loads it — so a broken atlas entry is reported as the atlas entry's fault instead of installing and failing later against your own files — and only then swaps it into place. `--force` *replaces* the target rather than merging into it, so no install can leave a hybrid corpus behind.
