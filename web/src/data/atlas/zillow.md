---
name: Zillow
slug: zillow
site_url: https://www.zillow.com/
domains: [zillow.com, www.zillow.com]
description: Zillow's signed-in landing page — search bar, top navigation, and the carousel of recommended homes.
categories: [commerce]
author: chiplay
created: 2026-08-08
updated: 2026-08-08
last_verified: 2026-08-08
cli_version: 0.19.0
spec_version: 1
method: browser
auth: personal-account
---

# Zillow

One view: the signed-in landing page. 9 components, every selector counted
against the live page.

Only the landing page is mapped. Listing detail pages carry home addresses,
owner-adjacent detail, and agent phone numbers, and mapping them would mean
handling a large amount of personal data about people who are not the account
holder. That is a deliberate boundary, not an omission.

## Why this is `auth: personal-account`

The carousel on this page is a set of homes recommended to the account. It is
mapped as structure and nothing inside it is recorded.

Only the author can re-verify this entry. `last_verified` means the author
re-checked with their own account; CI cannot, and neither can a reviewer.

There are no screenshots. Every frame of this page shows real homes.

## What bites

**A prefix selector double-counts here.** `[data-testid^="home-rec-card-"]`
matches sixteen elements, not eight, because `home-rec-card-anchor-N` shares the
prefix with `home-rec-card-N`. Excluding the anchors is what gets you the card
count.

**The anchor is a sibling of its card, not a child.** Scoping the anchor inside
the card matches nothing — which is the other half of why the shared prefix is
confusing.

**Cards carry indexed test ids**, `home-rec-card-0` upward, so no fixed test id
addresses a card. Only the prefix is stable.

**There is no `main` element.** A query for `main` matches nothing; the top nav
and the carousel are the landmarks.

**Badge counts overstate.** `[data-testid="property-card-badge"]` matches eleven
against eight cards, because three sit outside the carousel. Scoped to a card
there is exactly one each.

**Three inputs, one visible.** Scope the search field to the search-bar
container rather than querying inputs globally.

**Card links embed a street address.** The anchor href is
`/homedetails/:addressSlug`, and the slug is the address of a real home. The
property naming that address is declared because naming what a selector yields
is allowed, but the value is personal data about a third party and should be
treated as such.

## Coverage

9 selectors counted on the route that declares them, all matching. One was
wrong on the first pass and caught before commit — the card anchor, scoped as a
child of the card it belongs to, matched nothing.

No requests are recorded. `sightmap sel-probe` cannot attach to the browser this
was authored in, so matches were counted in-page instead.
