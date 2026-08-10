---
name: Zillow
slug: zillow
site_url: https://www.zillow.com/
domains: [zillow.com, www.zillow.com]
description: Zillow mapped signed in — landing page, search, listing detail, saved homes and the 404, and their three incompatible card conventions.
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

# Zillow

Five views — the signed-in landing page, search results, a listing, saved homes,
and the 404. 59 components, 4 requests.

## Why the map matters

Three routes list homes and no card selector works across all three. Worse, the
search page's card test id is reused on a listing page, where it matches five
*other* homes in the similar-homes rail rather than the one being viewed — so
the wrong answer looks exactly like the right one.

The two facts an agent most wants from a listing, price and address, have no
test id at all.

## Try it

```bash
sightmap atlas add zillow
```

Search, listings and the 404 work signed out; the landing carousel and saved
homes need an account.

## What bites

- **Three routes, three card conventions** — and on a listing page,
  `property-card` means the similar-homes rail, not the listing.
- **Price and address have no test id.** Read them from the `h1` and the title.
- **`data-price-row` is not a row.** It is a `span` holding one price; the real
  row has no test id, so the history columns must be zipped by index.
- **Half a listing page renders twice**, header facts and media tabs included.
- **Search filters are path segments, not query parameters.**
- **The 404's `document.title` is the empty string** — the only route where
  that is true, and the cheapest test for it.

## Personal data

Listing pages carry home addresses and agent contact details. Selectors and what
they mean are recorded; values are not. No address, price, agent name or phone
number appears in the corpus. The two screenshots show public for-sale listings
as Zillow publishes them.

Only the author can re-verify this entry — CI cannot, and neither can a
reviewer.
