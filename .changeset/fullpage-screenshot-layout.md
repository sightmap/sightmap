---
"@sightmap/sightmap": patch
---

Full-page screenshots (`capture --screenshot`, `capture --inbox`, `browser screenshot --full-page`) now keep the page's layout. `captureBeyondViewport` alone kept the window's width, so with classic scrollbars the page relaid out 15 px wider for the shot: text rewrapped and a capture's element boxes drifted from the screenshot down the page (65 px by y 7600 on a 26000 px article). Chrome also left the tab at that wider layout afterwards. The viewport is now pinned to the page's client width and document height, with scrollbars hidden, for the screenshot, then restored along with the scroll position.
