---
"@sightmap/sightmap": minor
---

Add `sightmap device network list|get`: the iOS app's requests in the local Simulator, matched against the corpus's `requests:` and with their `properties:` extracted, like `sightmap browser network list|get`. `device start --network auto|har|rn-cdp|summary|off` picks the source: CFNetwork's HAR log (full URL, headers and bodies; needs the `get-task-allow` entitlement, which Debug builds signed for development have), Metro's DevTools proxy for a React Native Debug build, or CFNetwork's summary log (status, timing and host only). `status` and `doctor` report the active source and why the others are not used. Credentials in headers, query strings and bodies are redacted unless `--unredacted`.
