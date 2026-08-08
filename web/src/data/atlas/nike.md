---
name: Nike
slug: nike
site_url: https://www.nike.com/
domains: [nike.com]
description: US storefront — the shared product wall behind both search and category browse, product pages, and a merchandising 404.
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

# Nike

The US storefront, mapped as a signed-out shopper.

## Coverage

| View | Route | What it holds |
|---|---|---|
| `Home` | `/` | Editorial campaign cards and product carousels |
| `SearchResults` | `/w` | Product wall — filter rail, result grid |
| `CategoryWall` | `/w/:categorySlug` | The same wall, addressed by category |
| `ProductDetail` | `/t/:productSlug/:styleCode` | Carousel, price, size grid, add to bag |
| `NotFound` | `/**` | Apology line above a product carousel |

`GlobalNav`, `NavScrim`, `ContentWrapper`, `ModalRoot`, `RouteAnnouncer`,
`EmbeddedMessaging`, and `GlobalFooter` are global. `PromoBanner` renders on
`ProductDetail` only. All five views report `0 orphaned` coverage, with every
selector `sel-probe`d live against all five view URLs on 2026-08-08.

## Hazards

**Search and category browse are the same page.** `/w?q=<terms>` and
`/w/<categorySlug>` both render `#Wall` with the same `.product-card` grid and
the same filter rail. They are mapped as two views because the routes differ, but
one set of selectors drives both. The category slug ends in an opaque taxonomy
code (`37v7jznik1zy7ok`) that cannot be constructed from a name — follow a link
from the shopping menu.

**A discontinued product returns 200, not 404.** The page renders `pdp-main` and
shows "THE PRODUCT YOU ARE LOOKING FOR IS NO LONGER AVAILABLE" above a
recommendations carousel. Nothing structural separates it from a live product.
Check for `#pdp_product_title` and a size grid; both vanish when the product is
gone.

**The 404 sells things.** It is rendered by the same CMS app as the home page —
both mount `#ciclp-app` — and carries a carousel of real products at real prices.
An agent scraping "products on this page" gets a non-empty answer from a page
that found nothing. The title is the signal.

**Two test-hook attributes, and they are not interchangeable.** `data-testid`
covers the nav, footer, and product page; `data-test` appears on the browse shell
(`[data-test="browseShell"]`). Casing is inconsistent within a page too:
`pdp-main` and `atb-button` sit alongside `ImageCarousel` and `Thumbnail-0`.

**Ids that are not addressable.** React `useId` emits `#:r0:`, which changes per
render and is not a valid CSS identifier unescaped. Feature modules
(privacy consent, chat, AI search, error modal) each mount into a div whose id is
a random UUID, with the module's name appearing only on the paired
`#<name>-module-script-<uuid>` tag.

**Sold-out sizes stay in the grid.** Presence in `[data-testid="pdp-grid-selector-item"]`
is not availability.

## Lint

`sightmap lint` passes with no warnings, but one selector is written oddly and
the reason should be on the record. `EmbeddedMessaging` uses
`[id="embedded-messaging"]` rather than `#embedded-messaging`. The two match the
same single element both live and offline; the hash form trips the
`id-hash-selector` rule, which reads this as an auto-generated id. It is not —
it is the fixed container Salesforce's messaging widget mounts into, stable on
every route and every load observed here. The ids on this site that genuinely
are generated are the UUID module mounts described above, and nothing in this
corpus selects on them.

## Known gaps

Cart and checkout stop at the add-to-bag control.

Nike Members surfaces and anything behind Sign In are out of scope per
[docs/POLICY.md](../../docs/POLICY.md).

`ProductDetail` is pinned to one colorway. The style code ending the URL is the
colorway rather than the product, so a different color of the same shoe is a
different URL with the same slug. The page structure holds across both.

The wall renders 24 cards and appends on scroll. Only the first batch was mapped,
which is enough since every card is the same component.

## Screenshots

Signed-out views of public catalog pages.
