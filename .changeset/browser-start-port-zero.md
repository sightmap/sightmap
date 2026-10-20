---
"@sightmap/sightmap": patch
---

Fix `sightmap browser start --port 0` / `--cdp-port 0` (documented as
"auto-allocate"): port 0 now asks the OS for a free port and records the port
actually bound in `.sightmap/.session`. Previously 0 resolved to 0, the daemon
bound an OS-chosen port but wrote `serverPort: 0`, CDP landed on port 1, and
`start --detach` exited 0 while every client command then refused the session
with "no running session". `start --detach` also no longer reports a session
ready until its file records a reachable server port.
