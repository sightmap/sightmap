---
name: Apple
slug: apple
site_url: https://www.apple.com/
domains: [apple.com]
description: Editorial product pages and the configure-and-buy store, two systems sharing one header, plus a 404 that changes language with the URL.
categories: [commerce]
author: chiplay
created: 2026-08-07
updated: 2026-08-08
last_verified: 2026-08-07
cli_version: 0.18.0
spec_version: 1
method: browser
auth: none
---

# Apple

apple.com, mapped as a signed-out visitor.

## Coverage

| View | Route | What it holds |
|---|---|---|
| `Home` | `/` | Promotional tiles and a media gallery |
| `SearchResults` | `/:locale/search/:query` | Tabbed results across four verticals |
| `ProductMarketing` | `/:productSlug` | Local nav, marquee, scroll galleries |
| `BuyFlow` | `/**/shop/buy-mac/:model` | Stepped configuration with a running price |
| `NotFound` | `/**` | Apology, search box, sitemap link |

`GlobalHeader`, `NavCurtain`, `PageRoot`, `MainRegion`, `AnnouncementPortal`,
`NavSearchResultCount`, and `GlobalFooter` are global. All five views report
`0 orphaned` coverage, with every selector `sel-probe`d on 2026-08-07.

## Hazards

**The 404 changes language with the URL.** The first path segment is a locale
code, so `/fr/nope-xyz` returns "Page introuvable", `/no/nope-xyz` returns
Norwegian, and `/zzz/nope-xyz` returns English. An agent that detects errors by
matching the message text gets a different string depending on a path segment it
may never have chosen deliberately. Match on structure instead.

**A single-segment unknown path is indistinguishable from a product page.**
Marketing pages live at `/:productSlug`, so `/no-such-page` has the same route
shape as `/macbook-air`. The structural tell is `#ac-localnav`, which every
marketing page carries and the error page does not.

**Marketing pages cannot sell anything.** `/macbook-air/` is editorial — a
starting price and a link, with no configurator and no add-to-cart. The purchase
surface is `/shop/buy-mac/macbook-air`. These are two different systems sharing
one header, and confusing them is the easiest mistake on this site.

**Most store URLs you find by crawling are aliases.** There is a permalink layer
at `/us/shop/goto/` using underscores that redirects to the hyphenated canonical
route. The home page alone carries over a hundred of them.

**Two ids for one footer.** `#ac-globalfooter` on the landing and marketing
pages, `#global-footersection` on search and the store. Matching only one finds
the footer on about half the site.

**Buy-flow options are ordered steps, not fields.** Later steps depend on earlier
ones, so a configuration cannot be filled in arbitrary order. The sticky bar
carries the running total, which diverges from the header's starting price as
soon as anything is selected.

**Ids that are not addressable.** React `useId` produces `#_r_3_` for search tab
panels and `#announce-message-_r_8_` for the nine live regions inside `#portal`.
They change per render and are not valid CSS identifiers unescaped.

**Marketing pages are enormous and video-driven.** `/macbook-air/` is roughly
28,000 pixels tall, with paired `-startframe-` and `-endframe-` images standing
in for video until it plays. A snapshot at the top captures placeholders.

## Known gaps

The buy flow is mapped through configuration and stops before the bag. Going
further means starting a real purchase.

Apple Account, order status, and anything behind sign-in are out of scope per
[docs/POLICY.md](../../docs/POLICY.md).

Only the US locale is mapped. Since the first path segment selects locale, other
markets serve different prices, availability, and copy from the same structure.

`ProductMarketing` and `BuyFlow` are each pinned to one product. The `rf-bfe-`
and `.section` vocabularies hold across the range; which steps and sections
appear does not.

`SearchResults` was mapped on the Explore tab. The other three tabs
(Accessories, Support, Find a Store) use the same shell and were not populated.

## Screenshots

Signed-out views of public pages.
