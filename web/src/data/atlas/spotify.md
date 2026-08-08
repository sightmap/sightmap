---
name: Spotify
slug: spotify
site_url: https://open.spotify.com/
domains: [open.spotify.com]
description: Spotify's web player album view, mapped signed in — album header, track list, and the playback chrome around them.
categories: [media]
author: chiplay
created: 2026-08-08
updated: 2026-08-08
last_verified: 2026-08-08
cli_version: 0.19.0
spec_version: 1
method: browser
auth: personal-account
---

# Spotify

One view so far: an album page on the web player, mapped through a signed-in
browser. The album itself is public; the account only supplies the session.

## Why this is `auth: personal-account`

The web player renders the same chrome whether or not you are signed in, but the
signed-in build is the one people actually delegate to, and it is what this map
describes. Nothing about the account ships. The library grid, the account menu,
and the playback bar are documented as structure — the map records that they
exist and what shape they have, never what is in them.

Only the author can re-verify this entry. `last_verified` means the author
re-checked with their own account; CI cannot, and neither can a reviewer.

There are no screenshots. Every frame of this view carries the account's library
down the left rail and its current track along the bottom, and the rule is to
drop a screenshot rather than retouch one.

## What bites

**The document title is the playing track, not the page.** Over the course of
mapping a single album page the title read three different things, none of them
the album. Anything using `document.title` to confirm a navigation will be wrong
whenever audio is playing. Read `[data-testid="entityTitle"]` instead.

**The `main` landmark's `aria-label` is stale.** It holds the previously viewed
album's name after a client-side navigation, so it disagrees with the page it
labels.

**Three visible `h1` elements.** They come from the library rail, the album
header, and the playback bar. A bare `h1` query picks one of the three
arbitrarily.

**Album artwork is not where it looks.** Neither `cover-art-image` nor
`entity-image` matches anywhere inside `[data-testid="album-page"]`. All three
`cover-art-image` nodes belong to the playback widget, so an agent reaching for
"the album cover" gets the currently playing track's art — wrong content, and
account state rather than page structure.

**The account menu is in the nav rail, not the top bar**, which is the reverse
of where the layout puts it. The library grid is a root-level sibling of the
nav, inside neither.

**A hidden language dialog ships on every page**, contributing 74
`language-option-*` testids that never become visible. Counting testids
overcounts by that much.

## Coverage

25 components across one view, every selector verified against the live page.
`sightmap sel-probe` cannot reach a browser the extension drives, so the
selectors were counted in-page instead; two were mis-scoped on the first pass
and both are corrected.
