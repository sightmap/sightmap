---
"@sightmap/sightmap": minor
---

`sightmap browser start --detach` runs the daemon in the background (in its own session) and returns once it is serving, so scripts and agents no longer hang on the foreground daemon. Unlike `nohup start &`, the detached daemon survives the launching shell (it `setsid`s into its own session). `browser start` also now prints that it is a foreground daemon holding the shell, and `browser status` probes the sightmap HTTP server in addition to Chrome's CDP — reporting `⚠ degraded` when CDP is up but the server has been reaped, instead of a misleading `running`.
