---
"@sightmap/sightmap": minor
---

browser: enable CDP focus emulation (`Emulation.setFocusEmulationEnabled`) on each tab so `document.hasFocus()` is always true regardless of the real OS window focus. Some page widgets gate behavior on page focus — e.g. react-aria date pickers open only on focus *while the page holds OS focus* — which an agent-, side-panel-, or headless-driven page never has, making them impossible to actuate in-page. Matches Playwright/Puppeteer, which enable the same emulation by default. Best-effort: a transitional target or very old Chrome that rejects the call simply falls back to the prior focus-dependent behavior.
