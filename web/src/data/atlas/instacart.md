---
name: Instacart
slug: instacart
site_url: https://www.instacart.com/
domains: [instacart.com]
description: Instacart mapped signed in — storefront, aisle, product, in-store search and the 404, plus the GraphQL operations behind them.
categories: [commerce]
author: chiplay
created: 2026-08-08
updated: 2026-08-10
last_verified: 2026-08-08
cli_version: 0.19.0
spec_version: 1
method: browser
auth: personal-account
---

# Instacart

Five views — a retailer storefront, an aisle, a product, in-store search, and
the 404. 31 components, 8 requests.

## Why the map matters

The storefront never finishes loading. Skeleton placeholders keep mounting and
real items don't, so an agent that reads it comes away with a handful of
products and no signal that it is looking at a fraction of the shelf. The map
says so, and says to read items from an aisle or a search instead.

Underneath, everything is one `/graphql` endpoint — but the operation name is in
the query string, so a network log is filterable without parsing a single body.

## Try it

```bash
sightmap atlas add instacart
```

Sign in and pick a retailer first; nothing renders without a delivery address.

## What bites

- **The storefront never resolves.** Placeholders outnumber items and the gap
  widens the longer you wait. Aisle, product and search all resolve normally.
- **Scripted scrolling mounts nothing.** `window.scrollTo` moves the viewport;
  only a real scroll event loads the next row.
- **Item cards are addressed three different ways** depending on route. The
  product link is the only handle that works everywhere — and it carries
  `role="button"`, so accessibility-tree queries for links miss every item.
- **Price has no test id and no stable class**, anywhere.
- **The cart is a drawer, not a route.** It sits in the DOM whether open or
  not, one of eight `[role="dialog"]` elements.

Two screenshots, captured signed in with the delivery address replaced. Only the
author can re-verify this entry — CI cannot, and neither can a reviewer.
