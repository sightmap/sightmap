---
name: Vuori
slug: vuori
site_url: https://vuoriclothing.com/
domains: [vuoriclothing.com]
description: Shopify storefront on Next.js — collections, product pages, search, and a 404 with an empty document title.
categories: [commerce]
author: chiplay
created: 2026-08-06
updated: 2026-08-06
last_verified: 2026-08-06
cli_version: 0.18.0
spec_version: 1
method: browser
auth: none
---

# Vuori

A direct-to-consumer apparel storefront, mapped as a signed-out shopper. Shopify
behind a Next.js front end with MUI components, which makes it a useful contrast
to the big-box entries in the atlas: the same shopping surfaces, built from an
entirely different stack.

## Coverage

| View | Route | What it holds |
|---|---|---|
| `Home` | `/` | Hero carousel, featured products, tabbed category carousel |
| `Collection` | `/collections/:handle` | Headline, subcategory carousel, filter bar, product grid |
| `ProductDetail` | `/products/:handle` | Media grid, title and price, options, spec accordions |
| `SearchResults` | `/search` | The same filter bar and grid as a collection |
| `NotFound` | `/**` | Apology heading, empty title |

`PromoBanner`, `DesktopHeader`, `MobileDrawer`, `MainContent`, `Breadcrumbs`,
`SkipNavigation`, `RouteAnnouncer`, `CookieConsent`, `NewsletterOverlay`,
`GlobalFooter`, `FeedbackWidget`, and `BuildStamp` are global. All five views
report `0 orphaned` coverage, with every selector `sel-probe`d on 2026-08-06.

## What bites

**The 404 has an empty document title.** Not a message, not a code — `""`. Any
agent that identifies pages by title gets nothing, and code branching on a title
containing "404" or "not found" never fires. The only text signal is the h1,
"The page you're looking for cannot be found", which also avoids the number. An
unknown collection handle lands here too rather than redirecting.

**Some `data-testid` values are content, and one kind is unusable.** Carousel
slides are labelled with the product name and colourway
(`data-testid='Seaside Short 8" | Nautical'`) and category cards with the
category label (`data-testid="Athletic Shorts"`). They change whenever
merchandising does, and the product ones contain a double quote for the inch
mark, so they cannot be written as a CSS attribute selector without escaping.
Treat any testid that reads like a sentence as data.

**The skip-navigation link is broken.** Its href resolves to
`/products/meta-pant-oxblood.json#main-content` rather than to `#main-content` on
the current page, so following it navigates to an unrelated product's JSON
endpoint. Present on every route, including the 404.

**The page prints its own render time.** `#buildTime` holds a timestamp that
changed on every load observed, which makes it a per-request server-render time
rather than a build stamp — a direct read on whether a response was cached or
freshly generated.

**The product card is the anchor.** There is no inner link to descend into; the
href is on `[data-testid="productCard"]` itself. Card text runs badge, name, and
price together with no separator.

**Collection handles resolve loosely.** `/collections/mens-shorts` serves
`/collections/shorts` and rewrites the path, so read
`[data-testid="plp-headline-container"]` rather than trusting the URL.

**The mobile drawer renders on desktop.** It ships closed at full viewport size
and contains its own copy of the whole navigation, so menu links match twice on
every route.

## Known gaps

Cart and checkout stop at the product options. Going further means adding items
to a real cart on a live storefront.

Accounts, order status, and anything behind Sign In are out of scope per
[docs/POLICY.md](../../docs/POLICY.md).

`MediaGrid` uses `#media-grid-desktop`, a desktop-only id. The mobile gallery is
a separate element, so this corpus describes the desktop viewport. Everything
else mapped here is viewport-independent.

`ProductDetail` is pinned to one colourway, since the handle encodes colour and
each colour is its own URL.

## Screenshots

Signed-out views of public catalog pages.
