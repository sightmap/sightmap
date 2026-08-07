---
name: eBay
slug: ebay
site_url: https://www.ebay.com/
domains: [ebay.com]
description: Marketplace search, category browse, item detail, and the live-shopping hub, mapped as a signed-out shopper.
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

# eBay

Everything an agent can reach on ebay.com without an account: find something,
narrow it down, and read the listing.

## Coverage

| View | Route | What it holds |
|---|---|---|
| `Home` | `/` | Category flyout, live-shopping carousel, rotating promo modules |
| `SearchResults` | `/sch/i.html` | Filter rail, result river, sort and pagination controls |
| `CategoryBrowse` | `/b/:categoryName/:categoryId/:browseNode` | Breadcrumbs, category tree, refinements, item grid |
| `ItemDetail` | `/itm/:itemId` | Gallery, title, price, quantity, buy actions, shipping, returns |
| `LiveHub` | `/ebaylive` | Filterable feed of live and upcoming shopping streams |
| `NotFound` | `/**` | Catch-all error page |

`GlobalHeader`, `GlobalFooter`, `GlobalPromoBanner`, and `FloatingHelp` are
global components. All six views report `0 orphaned` coverage, with every
selector `sel-probe`d on 2026-08-06.

## What bites

**A category URL can lie about which category you are on.** The name segment in
`/b/:categoryName/:categoryId/:browseNode` is decorative.
`/b/Totally-Made-Up-Name/220/bn_1865497` loads Toys & Hobbies and quietly
rewrites the path to the canonical name. A mismatched name and id pair is worse:
`/b/Cordless-Drills/184644/bn_1853799` serves Porcelain Nativity Items with a 200
and no warning at all. Read `.textual-display` after the page loads; the URL you
requested proves nothing.

**Three surfaces list products and no two share a card class.** Search uses
`li.s-card`, category browse uses `.brwrvr__item-card`, promo carousels use
`.brw-product-card`, and eBay Live uses `.dp-subgrid-item`. There is no single
card selector, and reaching for one is how a scraper silently returns nothing on
half the site.

**Scope result cards or overcount them.** The bare `.s-card` class also matches
sponsored and related-item cards sitting outside the results list, so
`ul.srp-results > li.s-card` is the selector that returns the 60 results the page
actually has. The same applies to `a.s-card__link`, which matches once per
carousel thumbnail and lands near 150 on a 60-result page until you exclude
`.image-treatment`.

**Filter group ids are positional and shift with the query.** `#x-refine__group__0`
through `__8` are assigned in render order, so the price group is not reliably
group 4. Match on the `.x-refine__item__title` heading text instead. Buying
Format, Condition, and Item Location are not in the left rail at all; they render
as menus in the controls bar.

**Class names split into two families and only one is safe.** Prefixed names
(`gh-`, `srp-`, `brw-`, `x-refine__`) are structural and survive deploys. Bare
short names like `.esJT`, `._5hsT`, and `.Jd4g` are compiled output. So are the
home page's `s0-0-1-1-0-2-…-container` ids, which encode a module's position in
one particular render.

**A cold profile gets a proof-of-work splash first.** The first request lands on
`/splashui/challenge`, the browser answers it with `argon2.wasm`, and then the
real URL loads. Nothing is required of the agent, but the first page is slower
and `document.referrer` points at the challenge rather than wherever you came
from.

## Known gaps

Cart and checkout are mapped only as far as the buy-box buttons on
`ItemDetail`. Going further means adding items to a cart and moving toward a
purchase on a live marketplace, which is not something a map should do to
someone else's storefront.

Signed-in surfaces (My eBay, Watchlist, purchase history) are out of scope per
[docs/POLICY.md](../../docs/POLICY.md). The header links to them from every
route, so an agent following the global nav will hit a login wall there and
nowhere else.

`ItemDetail` is pinned to one real listing, and listings end. When that URL stops
resolving, re-probe against any current `/itm/` id; nothing in the view depends
on that particular item.

## Screenshots

Both are signed-out views of public catalog pages. The search screenshot shows a
"Shipping to" ZIP that eBay inferred from the capture machine's IP address, which
is the store's regional default rather than anything a visitor entered.
