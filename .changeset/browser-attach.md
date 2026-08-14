---
"@sightmap/sightmap": minor
---

`browser start --attach host:port` attaches the daemon (devtools server +
console/network collector) to an already-running Chrome's CDP endpoint instead
of launching and owning its own. It is a deliberately degraded mode: no owned
profile or extension guarantees, capture is complete only from attach onward
(pre-attach network/console history can't be recovered — `Network.enable`
replays nothing), and `browser stop` detaches rather than killing the
caller-owned browser. The collector and devtools query surface are unchanged —
the collector only ever needed a CDP address — so live console/exception and
request matching work identically once attached.

Also fixes a latent shutdown-order bug in the collector surfaced by attach mode:
`Collector.Stop` cancelled the per-tab drain contexts *after* `wg.Wait()`, which
deadlocked whenever the CDP connections were still healthy at stop time (the norm
when a consumer stops the collector without tearing down the browser). Stop now
cancels the drains first and refuses new tab attachments during teardown.

And fixes extension-server-port injection under attach: `findExtensionSWWS` took
the *first* extension service worker it found, so in a real browser with several
extensions installed (the attach case) it injected the port hint into the wrong
one and the sightmap overlay never learned the server port — falling back to
probing 7891–7900 and latching onto whatever sightmap server answered first
(often a different session). It now identifies the sightmap extension by its
manifest name before injecting, so the correct overlay gets the hint. An owned
launch never hit this: its isolated profile contains only the sightmap extension.
