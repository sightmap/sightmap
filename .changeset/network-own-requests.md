---
"@sightmap/sightmap": patch
---

`sightmap network` no longer fills with the tool's own traffic. The extension's content script polled `/sightmap/version` every 4 s and each poll was buffered; a long session overflowed the 1000-entry ring ("8890 earlier network entries dropped") and the app's own `/api` requests were gone. The collector now drops requests to the session's sightmap server port, the server's `/sightmap` and `/sightmap/version` endpoints on the loopback ports 7891-7900 (the extension's port probing), and `chrome-extension://` URLs at capture, so they cannot displace the page's requests. The content script also stops polling while the overlay is hidden, which is always the case in a CDP-driven or headless session (nothing reads the corpus then); opening the side panel polls once to catch up.
