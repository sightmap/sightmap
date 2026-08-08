---
name: Instacart
slug: instacart
site_url: https://www.instacart.com/
domains: [instacart.com, www.instacart.com]
description: Instacart's retailer storefront, mapped signed in — aisle rail, item cards, cart entry, and the skeletons that never resolve.
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

# Instacart

One view: a retailer storefront. 10 components, every selector counted against
the live page.

## Why this is `auth: personal-account`

The storefront only renders against a chosen retailer and a delivery address,
which means a signed-in session. The header carries that address, the fulfilment
window, and the cart count. All three are account data and none of them ship.
The map records that the controls exist and what shape they have.

Only the author can re-verify this entry. `last_verified` means the author
re-checked with their own account; CI cannot, and neither can a reviewer.

There are no screenshots. The header shows the account's street address in every
frame, and the rule is to drop the frame rather than retouch it.

## What bites

**The page never finishes loading in a background tab.** Rows below the first
mount as skeleton placeholders and stay that way. After scrolling, 45
placeholders stood against 6 real items, and the placeholder count *rose* as
more sections mounted. Nothing about this is a timeout you can wait out — at
thirty seconds it was worse than at eight.

**Scripted scrolling does not trigger the lazy load.** `window.scrollTo` moved
the viewport and mounted nothing. A real scroll event rendered the next row.
Drive this page with input events, not with injected script — the distinction
decides whether you see one product row or several.

**There is an inverted settle signal.** `[data-testid^="loading-lockup"]` counts
the unresolved sections. A route is fully read when that count is zero, and on
this storefront in a background tab it never reaches zero.

**No fixed test id addresses an item.** They embed the item id, as in
`item_list_item_items_45295-16498490`. Match on the `item_list_item_` prefix.

**The header holds two inputs and one is a decoy** — a hidden shim for the iOS
virtual keyboard. An unfiltered `input` query inside the header returns two.

**Class names are generated** (`e-xdho1a`, `e-v26ry7`) and change between
builds. Select on ids, test ids, or `aria-labelledby`.

**Section headings and the store name are both `h2`**, so a heading query
returns the store alongside the product rows.

## Coverage

10 selectors counted on the route that declares them, all matching. No requests
are recorded: the network capture for this tab returned a single third-party
beacon across two page loads, so rather than guess at API routes the dimension
is left empty and marked here as a known gap.

`sightmap sel-probe` cannot attach to the browser this was authored in, so
matches were counted in-page instead.
