---
"@sightmap/sightmap": patch
---

Remove the authoring skill's instruction to run `sightmap browser register --addr localhost:PORT` — that subcommand does not exist, so agents following it dead-ended. Attaching to an externally-launched browser remains a possible future feature (tracked upstream); until it lands the skill no longer documents it.
