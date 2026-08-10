---
name: IKEA
slug: ikea
site_url: https://www.ikea.com/us/en/
domains: [ikea.com]
description: US storefront — search, category browse, product detail, and the CMS-driven landing page, mapped signed-out.
categories: [commerce]
author: chiplay
created: 2026-08-06
updated: 2026-08-08
last_verified: 2026-08-08
cli_version: 0.18.0
spec_version: 1
method: browser
auth: none
---

# IKEA

The US storefront (`/us/en/`), mapped as a signed-out shopper.

## Coverage

| View | Route | What it holds |
|---|---|---|
| `Home` | `/us/en` | Category carousel and a stack of CMS-driven promo modules |
| `SearchResults` | `/us/en/search` | Result summary, filter sidebar, product grid |
| `CategoryBrowse` | `/us/en/cat/:categorySlug` | Page header, then either a subcategory carousel or the product grid |
| `ProductDetail` | `/us/en/p/:productSlug` | Gallery, price, ratings, fulfillment, add to cart |
| `NotFound` | `/**` | Catch-all page |

`GlobalHeader`, `Breadcrumbs`, `GlobalFooter`, `MainContent`, `CookieConsent`,
`PreloadedModalHosts`, and `FeedbackPrompt` are global. All five views report
`0 orphaned` coverage, with every selector `sel-probe`d at authoring and
re-verified against the live site on 2026-08-08.

Five views is the whole public shopping surface. Cart and checkout are excluded
deliberately (see Known gaps), and the remaining top-level paths (`/rooms/`,
`/ideas/`, `/stores/`, `/offers/`) are editorial pages built from the same
`hnf-` frame and the same CMS modules the home page uses, so mapping them would
repeat `Home` under different names rather than describe new structure.

## Hazards

**Learn the four class prefixes and most selector guessing disappears.** `hnf-`
is the shared header, nav, and footer frame. `plp-` is the product list, shared
by search and category pages. `pipf-` is the product page. `crec-` is the
recommendation fragment that loads late. A `plp-` class on a `/p/` URL means you
are looking at a recommendation strip, not the product.

**The `plp-` grid is only shared down to the card.** `#product-list`,
`[data-testid="plp-product-card"]`, `[data-testid="plp-filter-side-bar"]` and
`.plp-price-module` behave the same on search and on a category. Inside the card
they do not: search renders `.plp-grid-product-card__*` and a category renders
`.plp-mastercard__*`. A price selector written against a search result returns
nothing on a category page and raises no error. Use `.plp-price-module`.

**Half of `/cat/` has no products on it.** The same route serves a hub, which
carries the subcategory carousel and nothing to buy, and a leaf, which carries
the grid and filters and no carousel. `/cat/tables-chairs-fu002/` is a hub;
`/cat/desk-chairs-20654/` is a leaf. Nothing in the URL distinguishes them —
test for `#product-list` before concluding a category is empty.

**A category id you got wrong sends you to the entire catalog.** The slug is
decorative: `/cat/totally-made-up-fu002/` loads Tables & Chairs and rewrites the
path. But a bad *id* does not 404 either. `/cat/tables-chairs-zz999/` redirects
to `/cat/products-products/`, the root of the catalog, so a typo silently widens
a narrow query to everything IKEA sells. Read `.hnf-pageheader__h1` to find out
where you actually are.

**Five closed modals ship on every page.** Separate hosts from five different
frontends (`hnf-`, `wlo-modal-`, `rec-`, `ugc-rr-pip-fe-`, `rr-leave-review-`)
are all rendered before anything opens them, each carrying a `--close` modifier
and each with a paired `__backdrop`. Counting `[role=dialog]` or asking "is a
modal open" gives the wrong answer. Test for the absence of `--close`.

**The home page's module testids are CMS ids.** Each promo module carries an
outer `data-testid` that is a Contentful entry id
(`data-testid="5EPhOkZOk1TdhhrR5us7Yt"`) and an inner one that names the
component type (`hri-bento-container`, `hri-deals`, `hri-editorial-shelf`). Only
the inner one survives a marketing edit.

**Card counts are page sizes, not result counts.** The grid renders about 22
cards and appends on scroll. `.search-summary` carries the real total.

**Product names keep their diacritics.** GÖRSNYGG, PÄRKLA. Matching on an
ASCII-typed name fails.

**The article number is in the URL and in the API calls.** The digits ending a
product slug (`...-40504193/`) are the article number that
`web-api.ikea.com/circular/...` and the 3D model endpoint both key on, so API
calls are derivable from the URL with no lookup step.

## Known gaps

Cart and checkout stop at the add-to-cart control. Going further means putting
items into a real basket on a live storefront.

The store picker inside `FulfillmentSection` changes what the page reports for
delivery and in-store stock. It was left on the IP-inferred default rather than
driven, so availability text in this map reflects one location.

Search embeds a dormant Cloudflare Turnstile widget (`cf-turnstile-response`).
It never challenged during this mapping, but it makes search the route most
likely to start gating automated traffic, and this map would go stale there
first.

Only the `/us/en/` storefront is mapped. The locale prefix is part of the path,
so other markets have their own prices, availability, and category ids.

## Screenshots

Signed-out views of public catalog pages. The utility bar and fulfillment panel
name a nearby store and ZIP inferred from the capture machine's IP address — a
geolocated store default, not anything a visitor entered.
