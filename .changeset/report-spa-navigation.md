---
"@sightmap/sightmap": patch
---

Better handling of asynchronous SPA navigation, without baking implicit waits into `click`.

- **`navigate` now reports client-side redirects.** It previously only saw server-side (HTTP) redirects, which are reflected by the load event; a client-side redirect that fires during hydration (an auth guard bouncing `/login → /`, or `/ → /workspace`) went unreported, so the caller was told it was somewhere it wasn't. `navigate` now waits briefly after load for a follow-up navigation and prints `(redirected to FINAL)` for those too.
- **`wait-for` gains `--view` and `--component`.** These are the explicit, corpus-aware step boundaries to use after an action that should navigate (the act-then-wait split Playwright and Selenium use): `--view <Name>` waits until the current URL resolves to a named sightmap view, and `--component '<Query>'` waits until a component query — including property filters like `WorkItemRow[key="FALCON-7"]` — matches a node. Both auto-retry until they hold or time out loudly. `--url`, `--selector`, and `--load` remain the raw equivalents.

`click` deliberately does **not** wait for or guess about resulting navigation — it acts, reports, and keeps its loud covered/off-screen refusals. Adds `browser.AwaitNavigation` (waits for `Page.frameNavigated` / `Page.navigatedWithinDocument`, settling chained redirects) behind `navigate`.
