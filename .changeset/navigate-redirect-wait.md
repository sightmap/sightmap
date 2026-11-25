---
"@sightmap/sightmap": patch
---

`sightmap browser navigate` no longer waits a fixed 2 s after every page load for a client-side
redirect. When the corpus recognises the page, it watches for a redirect until the app has
rendered a known component and gone quiet, and for at least 1 s after load (a redirect from a
short timer fires after the first render), and with `--settle` that same quiet period is the
settle. A navigate on a fast local app drops from about 2.5 s to about 1 s, and a redirect the
app makes later than 2 s while still booting is now reported instead of missed. A page the
corpus doesn't recognise keeps the old 2 s watch.
