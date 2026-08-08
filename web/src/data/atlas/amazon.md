---
name: Amazon
slug: amazon
site_url: https://www.amazon.com/
domains: [amazon.com]
description: US marketplace — search, product detail, best-seller charts, and a text-free 404, mapped signed-out.
categories: [commerce]
author: chiplay
created: 2026-08-06
updated: 2026-08-08
last_verified: 2026-08-06
cli_version: 0.18.0
spec_version: 1
method: browser
auth: none
---

# Amazon

The US marketplace, mapped as a signed-out shopper.

## Coverage

| View | Route | What it holds |
|---|---|---|
| `Home` | `/` | Shoppable card deck and recommendation rails |
| `SearchResults` | `/s` | Refinement rail, 48-card result grid, pagination |
| `ProductDetail` | `/dp/:asin` | Three-column layout, buy box, feature slots |
| `BestSellers` | `/gp/bestsellers` | Department rail and ranked charts |
| `NotFound` | `/**` | Text-free error page |

`GlobalNav`, `KeyboardShortcutMenu`, and `PageRoot` are global. All five views
report `0 orphaned` coverage, with every selector `sel-probe`d on 2026-08-06.

## Hazards

**The 404 has no text at all.** `document.body.innerText` is the empty string.
The apology is the `alt` attribute of an image, the three links have image
content and empty text, and the markup is hand-minified with single-letter ids
(`#a` form, `#e` input, `#f` submit). An agent that detects errors by reading
page copy finds nothing and will most likely decide the page is still loading.
What works: `document.title === "Page Not Found"`, the absence of the entire
`nav-` chrome, and the image `alt` text.

**Ids are duplicated on the product page.** Amazon renders several buy-box
variants and hides all but one, so `#add-to-cart-button` and `#buy-now-button`
each match twice and `#availability` matches three times. Every duplicate sits
inside `#buybox` and the hidden ones measure 0×0. `querySelector` returns
document order, which is not a guarantee of visibility. There is also an
unrelated `#add-to-cart-btn` placeholder outside the page root, making three ids
in that family.

**Most of the product page is empty.** The page is built from slots whose ids end
`_feature_div`. On the article mapped here there were 541 of them and 501
rendered with zero height. A slot existing means the template reserved it, not
that it has content.

**`[data-asin]` is not the result selector.** It matches 88 elements on a search
page that has 48 results, because carousels, ad slots, and recommendation strips
carry ASINs too. Use `[data-component-type="s-search-result"]`, and read fields
through the `data-cy` attributes (`title-recipe`, `price-recipe`,
`reviews-block`, `delivery-recipe`), which appear exactly once per card.

**Ids that look stable and are not.** `#CardInstance<random>` on the best-seller
charts is regenerated per render. `#wd-shoppable-N` on the home page is
positional, so the number identifies a slot rather than a promotion.

**The page depends on where you are.** The header carries an IP-inferred delivery
location, and prices, availability, and delivery estimates all follow from it.
Two agents in different places get different pages at the same URL.

## Known gaps

Cart and checkout stop at the buy-box buttons. Going further means putting items
into a real cart on a live marketplace.

Signed-in surfaces (orders, lists, recommendations tied to an account) are out of
scope per [docs/POLICY.md](../../docs/POLICY.md).

`ProductDetail` is pinned to one ASIN. Feature slots vary enormously by
category, so a media or grocery article renders a substantially different set of
slots than the accessory mapped here. The three-column skeleton holds; the slots
inside it do not.

`SearchResults` and `BestSellers` were mapped from one query and the top-level
chart. Deeper chart departments use the same components.

## Screenshots

Signed-out views of public pages. The search screenshot shows the header's
IP-inferred delivery city and ZIP, which is Amazon's geolocation guess rather
than anything a visitor entered. The 404 screenshot is included because the page
is unusual enough to be worth seeing.
