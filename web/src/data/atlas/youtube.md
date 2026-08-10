---
name: YouTube
slug: youtube
site_url: https://www.youtube.com/
domains: [youtube.com, www.youtube.com]
description: YouTube mapped signed in — home, watch, search, channel, playlist and the 404, and the attribute that tells three of them apart.
categories: [media]
author: chiplay
created: 2026-08-08
updated: 2026-08-10
last_verified: 2026-08-08
cli_version: 0.19.0
spec_version: 1
method: browser
auth: personal-account
---

# YouTube

Six views — home, watch, search, channel, playlist, and the 404. 47 components,
5 requests.

## Why the map matters

YouTube's custom elements look like a clean API and aren't. Home, channel and
playlist are all `ytd-browse`; only an attribute separates them, so route
detection by element silently conflates three pages. Video rows ship under two
renderers at once, so counting either alone undercounts. And the selectors most
agents already carry — `input[name="search_query"]`,
`ytd-playlist-video-renderer` — match nothing at all now, returning empty lists
rather than errors.

## Try it

```bash
sightmap atlas add youtube
```

Sign in first if you want the home feed; watch, search and channel work either
way.

## What bites

- **Three routes share one element.** Home, channel and playlist are all
  `ytd-browse`, separated only by its `page-subtype` attribute.
- **The search field is a `textarea`**, not an input.
- **Video rows use two renderers at once.** Search carries both
  `ytd-video-renderer` and `yt-lockup-view-model`. On a playlist the legacy
  renderer matches nothing.
- **Subscribe is a different element on channel and on watch.**
- **The watch page hides duplicate copies of its own controls**, and for the
  like button the first match in document order is the hidden one.
- **Ids are not unique**, and the icon sprite owns generic ones like `#add`.

Three screenshots, captured signed in. Only the author can re-verify this
entry — CI cannot, and neither can a reviewer.
