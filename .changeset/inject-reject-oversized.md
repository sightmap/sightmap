---
"@sightmap/sightmap": patch
---

Fix `browser inject` (daemon `/inject`) silently truncating oversized uploads. The `maxInjectBytes` (16 MiB) cap was enforced with `io.LimitReader(r.Body, maxInjectBytes)`, which presents EOF to `io.ReadAll` on a body larger than the cap — so an oversized script was cut to exactly the cap, stored, and registered for re-injection on every tab while the daemon replied `200` with the truncated byte count. The handler now reads one byte past the cap and rejects bodies exceeding it with `413` ("script too large"), matching the cap+1 idiom already used by the atlas fetch/install readers.
