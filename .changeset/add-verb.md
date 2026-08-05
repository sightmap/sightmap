---
"@sightmap/sightmap": minor
---

Add `sightmap add <slug>`: install a published sightmap corpus from the community atlas (github.com/sightmap/atlas) into the current project, so one command from a gallery page yields a working `.sightmap/` directory. Looks the slug up in the atlas index (`--index` overrides the URL for mirrors and tests), suggests the closest slugs on a miss, fetches the entry's files pinned to its commit when one is published, and writes them into `--target` (default `./.sightmap`) — refusing non-empty targets without `--force`, rejecting any entry path that could escape the target, and requiring HTTPS everywhere except localhost.
