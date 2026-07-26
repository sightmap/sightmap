---
"@sightmap/sightmap": patch
---

Isolate concurrent browser sessions so agents working in different projects no longer cross-talk. The session file is now keyed to `--sightmap-dir` (it lives at `<sightmap-dir>/.session`) instead of a single shared `$TMPDIR` file, so a second `browser start` only ever reuses the Chrome for *its own* corpus rather than piggybacking on another agent's browser. Free-port probing now checks the IPv4 loopback (where Chrome's CDP and the sightmap server bind), and the sightmap server binds `127.0.0.1`, so two daemons started on the default ports slide to non-overlapping ports instead of one daemon's server colliding with another's CDP port. Every session-aware `browser` command (including `console`/`network`, `status`/`stop`, `tabs`, and the low-level interactions) now accepts `--sightmap-dir` to select which session it talks to.
