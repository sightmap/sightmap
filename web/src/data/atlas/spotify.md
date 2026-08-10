---
name: Spotify
slug: spotify
site_url: https://open.spotify.com/
domains: [open.spotify.com]
description: Spotify's web player mapped signed in — track, album, playlist, artist, home, search and the 404, with the playback chrome around them.
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

# Spotify

Seven views — track, album, playlist, artist, home, search, and the 404. 59
components, 4 requests.

## Why the map matters

The web player lies about where it is. Its document title is the currently
playing track, not the page, and the `main` landmark keeps the *previous*
album's label after a client-side navigation. Both of the obvious ways to ask
"what am I looking at?" return stale answers that look current.

Album, playlist, artist and track look interchangeable and aren't — each has a
different container and artist has none at all. And every view is fetched
through one POST endpoint, so the URL never says which page issued a call.

## Try it

```bash
sightmap atlas add spotify
```

Sign in first — the player renders the same chrome either way, but the
signed-in build is the one worth delegating to.

## What bites

- **The document title is the playing track**, not the page. Read
  `[data-testid="entityTitle"]` instead.
- **`main`'s `aria-label` is stale by design** after a client-side navigation.
- **Test the container, not the header.** `album-page`, `playlist-page` and
  `track-page` identify three routes; artist has none of them.
- **`[data-testid="track"]` is never a track.** It is an icon in the playback
  bar.
- **Signed out, the track route is a different build entirely** and matches
  nothing in this corpus — which is one way to notice a lapsed session.
- **Album artwork is not in the album.** Every `cover-art-image` on the page
  belongs to the playback bar, so reaching for it returns the playing track.
- **One POST endpoint serves every view**, with the operation named in the body
  — request-based view detection does not work here.
- **The 404 carries none of the chrome**, which is what identifies it.

Three screenshots, captured signed in. Only the author can re-verify this
entry — CI cannot, and neither can a reviewer.
