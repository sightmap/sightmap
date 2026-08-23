---
"@sightmap/sightmap": patch
---

`browser install` now fetches the correct architecture on **linux-arm64**. It previously mapped every Linux host to the `linux64` (x86-64) Chrome for Testing build regardless of CPU, so arm64 machines silently got an x64 Chrome under `~/.sightmap/browsers/chrome-<ver>/chrome-linux64/`. The platform is now resolved per-arch (`linux-arm64` on arm64, `linux64` on x64). Because Google does not currently ship an arm64 build to the **Stable** channel, resolution falls back through Beta → Dev → Canary for platforms Stable does not carry, printing a one-line warning when it uses a non-Stable channel. Every mainstream platform continues to install from Stable unchanged.
