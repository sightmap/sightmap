---
"@sightmap/sightmap": minor
---

Add `sightmap add <slug>`: install a published sightmap corpus from the community atlas (github.com/sightmap/atlas) into the current project, so one command from a gallery page yields a working `.sightmap/` directory. Looks the slug up in the atlas index (`--index` overrides the URL for mirrors and tests), suggests the closest slugs on a miss, fetches the entry's files pinned to its commit when one is published, and writes them into `--target` (default `./.sightmap`).

The atlas wire contract ships as a new public package, `github.com/sightmap/sightmap/go/atlas`, so the atlas publisher CI can enforce the same rules the CLI enforces instead of re-implementing them: the index schema and its version gate, the `<root>/<ref>/entries/<slug>/<path>` URL layout (one rule for GitHub raw, GitHub Enterprise raw, and a plain mirror), the fail-closed validators for slugs, commits, and corpus paths, a fetch client that holds its HTTPS-only policy on the requested URL and on every redirect hop and caps response size, and an `Install` operation.

`add` is fail-closed and atomic. It checks local preconditions before any network I/O, fetches every file (concurrently) before writing any, and stages the result beside the target before renaming it into place. Whatever the target held is moved aside rather than deleted until the installed corpus has been loaded — so a broken atlas entry is reported as the atlas entry's fault, with your previous corpus put back, instead of installing and failing later against your own files.

`--force` *replaces* the target rather than merging into it, so no install can leave a hybrid corpus behind — and because replacing is destructive, it is confined to directories that are visibly a corpus. A target that is the working directory, a directory above it, your home directory, or a filesystem root is refused whatever it holds, and so is one holding anything that is not corpus content (a `.git`, a `.env`, a `go.mod`, a `src/`). `--target . --force` in a project root is a refusal, not a way to delete the project.
