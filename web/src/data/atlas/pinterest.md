---
name: Pinterest
slug: pinterest
site_url: https://www.pinterest.com/
domains: [pinterest.com]
description: Pinterest mapped signed in — home feed, search, board, pin detail, profile and what a dead path renders instead of a 404.
categories: [social]
author: chiplay
created: 2026-08-08
updated: 2026-08-10
last_verified: 2026-08-08
cli_version: 0.19.0
spec_version: 1
method: browser
auth: personal-account
---

# Pinterest

Six views — home, search, board, pin detail, profile, and the path that
resolves to nothing. 59 components, 12 requests.

## Why the map matters

Pinterest fails quietly. A board that no longer exists redirects to the home
feed, so an agent following a stale link reads someone else's recommendations
as the board. A profile in a background tab renders its chrome and no content,
which looks the same as an account with nothing in it. And the obvious selector
for "the pin on screen" only ever matches the recommendation grid around it.

None of that raises an error. The map is what turns three silent wrong answers
into three known ones.

## Try it

```bash
sightmap atlas add pinterest
```

Sign in first — every route here needs an account.

## What bites

- **Server-rendered feeds fill in a background tab; fetched ones never do.**
  Home and search populate. A profile body, a board grid and a pin's related
  grid render their containers and stay empty.
- **There is no 404.** A missing board redirects to `/?show_error=true`. A
  missing user renders the same chrome-only page as a real profile.
- **`data-test-id="pin"` is never the pin being viewed**, on any route. It is
  always a card in a grid.
- **Card type is a test id, not an attribute.** `pincard-oneTap-*` marks a
  promoted card; the promoted share of a feed swings load to load.
- **Every `/resource/` call carries `source_url`**, so the route that issued a
  request is readable from its query string.

Three screenshots, captured signed in. Only the author can re-verify this
entry — CI cannot, and neither can a reviewer.
