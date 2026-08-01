---
"@sightmap/sightmap": patch
---

First-run papercuts fixed:

- Subcommand `--help`/`-h` now exits 0 and no longer prints `flag: help requested` — a help request is a success, not an error (#117).
- `browser status` on a session file that exists but doesn't parse (e.g. hand-written or corrupted, so no valid `port`) now reports it as an unrecognized format with the expected shape, instead of a misleading "server and CDP were assigned the same port (0)" collision hint. The collision hint is also gated on a real server port (#118).
- `validate` no longer warns about unknown fields in tooling files it owns (`survey.yaml`), as it already skips `config.yaml` (#119).
