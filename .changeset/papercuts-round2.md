---
"@sightmap/sightmap": patch
---

Three first-run papercuts:

- `validate` now errors on an unsupported `version:` value (e.g. `version: 2`) — the spec requires `version: 1`. This is the companion to the missing-`version:` warning.
- `browser install --help` prints usage and exits 0 instead of ignoring its arguments and starting the 184 MB Chrome-for-Testing download (the subcommand now parses its flags).
- `search` names the near-miss when a pattern matches a **view** or **request** name (search covers component fields only), instead of a bare "no matches".
