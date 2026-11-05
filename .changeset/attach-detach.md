---
"@sightmap/sightmap": patch
---

`browser start --attach HOST:PORT --detach` now backgrounds the attach daemon and returns once it is ready, instead of ignoring `--detach` and holding the shell. A reaped attach daemon can be restarted with `--detach` while the attached Chrome is still running.
