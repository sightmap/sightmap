---
name: YouTube
slug: youtube
site_url: https://www.youtube.com/
domains: [youtube.com, www.youtube.com]
description: YouTube's home feed and watch page, mapped signed in — feed grid, player, title row, description, comments, and related column.
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

# YouTube

Two views: the signed-in home feed and a watch page. 25 components, every
selector counted against the live page on the route that declares it.

## Why this is `auth: personal-account`

The home feed is assembled entirely for the account, and the watch page's
related column is too. Both are mapped as structure. Nothing about the account
ships: no watch history, no subscriptions, no recommendations, no identity.
Where a container holds account data, the map says the container exists and its
memory note says what is inside belongs to the account.

Only the author can re-verify this entry. `last_verified` means the author
re-checked with their own account; CI cannot, and neither can a reviewer.

There are no screenshots. Every frame of a signed-in session carries the
account's feed, and the rule is to drop the frame rather than retouch it.

## What bites

**The search field is a `textarea`, not an `input`.** It is
`textarea[role="combobox"]` inside `form[action="/results"]`. The selector
everyone reaches for — `input[name="search_query"]` — matches nothing. There
are only two `<input>` elements on the whole home page, both hidden checkboxes.

**Ids are not unique.** `#content` matched 46 elements on home and 12 on a watch
page. `getElementById` returns an arbitrary one of them. Select by element name
instead.

**The icon sprite owns generic ids.** `#add`, `#alarm`, `#accessibility` and
dozens more resolve to invisible sprite symbols rather than controls.

**The watch page renders two or three copies of its own controls** — title, like
button, description, comments — and hides all but one. Worse, for the like
button the *first* match in document order is the hidden one, so an unscoped
`querySelector` returns the wrong element. Scope to `#above-the-fold`, which
holds exactly one visible copy of each.

**No heading identifies the home route.** All four `h1` elements there are
empty. Test for `ytd-browse` instead.

**Comments start at zero.** `ytd-comment-thread-renderer` matches nothing until
the page is scrolled to the comment section. An empty count means not loaded,
not a video without comments.

**The guide drawer can render with no rows**, which means unpopulated rather
than missing navigation. Exactly one of the drawer and the mini guide is
visible at a time, and the hidden one still matches, so filter by visibility.

**Two mastheads exist.** The variant seen through most of this mapping carries a
Back button and no avatar button at all, at a 1768px viewport on both routes; a
variant with `#avatar-btn` appeared once and could not be reproduced. No account
control is declared here as a result.

## Coverage

Every selector was counted on the route that declares it — 19 on watch, 11 on
home, all matching. `sightmap sel-probe` cannot attach to the browser this was
authored in, so matches were counted in-page instead.

Home counts settle slowly: the feed grid read 3 items six seconds after
navigation and was still 3 at sixteen, having read 42 in an earlier session, so
treat feed-item counts as viewport- and session-dependent rather than fixed.
