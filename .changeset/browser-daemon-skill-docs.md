---
"@sightmap/sightmap": patch
---

Authoring + browser skills: document the browser daemon lifecycle. `browser start` is a long-running foreground daemon that holds the shell; scripts and agents should use `browser start --detach` (which returns once serving and survives the shell) rather than `nohup start &`. Also note headless auto-detection and the `--no-sandbox` hint on sandboxed hosts, and that `browser status` can report a `⚠ degraded` daemon.
