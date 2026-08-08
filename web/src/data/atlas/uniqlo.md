---
name: UNIQLO
slug: uniqlo
site_url: https://www.uniqlo.com/us/en/
domains: [uniqlo.com]
description: US storefront — category listings, product pages, search, and an error page with a real address.
categories: [commerce]
author: chiplay
created: 2026-08-07
updated: 2026-08-08
last_verified: 2026-08-08
cli_version: 0.18.0
spec_version: 1
method: browser
auth: none
---

# UNIQLO

The US storefront (`/us/en/`), mapped as a signed-out shopper.

## Coverage

| View | Route | What it holds |
|---|---|---|
| `Home` | `/us/en/:gender` | Media banners and a category grid |
| `SearchResults` | `/us/en/search` | Product tile grid |
| `CategoryListing` | `/us/en/:gender/:category` | Sticky subcategory tabs above the grid |
| `ProductDetail` | `/us/en/products/:productId/:colorCode` | Gallery, options, size table, related products |
| `NotFound` | `/us/en/not-found` | Error page, reached by redirect |

`PromoStrip`, `NavigationHeader`, `PageContent`, `Breadcrumbs`,
`PreloadedDialog`, `BackToFrontOverlay`, `Accordion`, `PromoModal`,
`ContentAlignment`, `ECGrid`, `Drawer`, `PopupFormDialog`, `AppShell`, and
`GlobalFooter` are global. All five views report `0 orphaned` coverage, with
every selector `sel-probe`d on 2026-08-07.

## Reliable signals

**The page type is a class on the content root.** `.fr-ec-template-plp` is a
category listing, `.fr-ec-template-pdp` is a product page, `.home-template` is
the landing page. An agent can branch on this instead of parsing a URL.

**The error page has a real address.** An unmatched path redirects to
`/us/en/not-found`, so `location.pathname` answers "did I land on an error page"
with no DOM inspection.

## Hazards

**The `data-testid` values are design-system primitives, not identifiers.**
`ITOTypography` matched 574 elements on one page, `ITOImage` 363, `ITOLink` 128.
They tell you an element is *a* piece of text and never *which* one. Use the
`fr-ec-` classes for identity and read `ITO*` as type information.

**The landing page has three addresses and state decides which you get.**
`/us/en/` redirects to whichever gender was browsed last, `/us/en/men` keeps its
path, and `/us/en/women` normalizes to `/us/en/`. The preference is stored
client-side, so two agents at the same URL land on different pages. This map
anchors on `/us/en/:gender` because that is where a warm profile actually ends
up.

**The navigation renders three times.** Desktop, mobile, and sticky variants all
sit in the DOM, so any count of nav links is tripled.

**Three empty overlay hosts ship on a browse route.** `ITODialog`,
`.b2f-overlay`, and the POPUP Form dialog are all present and empty before
anything opens them, so counting dialogs overstates what is actually showing.

**Breadcrumbs are at the bottom.** `[data-testid="ITOBreadcrumbGroup"]` sits at
the foot of the page, not above the content.

**A product URL grows after loading.** Requesting
`/us/en/products/:productId/:colorCode` appends `?colorDisplayCode=` and
`?sizeDisplayCode=` for the default variant, so the URL after load differs from
the one requested even on success.

## Known gaps

Cart, wishlist, and checkout are out of scope; the map stops at the product
options.

Accounts and anything behind My Account are out of scope per
[docs/POLICY.md](../../docs/POLICY.md).

`ProductDetail` is pinned to one colourway, since colour is a path segment rather
than a parameter.

Only the `/us/en/` storefront is mapped. The locale prefix is part of the path,
so other markets have their own catalogue and prices.

## Screenshots

Signed-out views of public catalog pages.
