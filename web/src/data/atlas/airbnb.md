---
name: Airbnb
slug: airbnb
site_url: https://www.airbnb.com/
domains: [airbnb.com]
description: Stay search with its live map, listing pages built from named sections, city landing pages, and a model 404.
categories: [travel]
author: chiplay
created: 2026-08-07
updated: 2026-08-07
last_verified: 2026-08-07
cli_version: 0.18.0
spec_version: 1
method: browser
auth: none
---

# Airbnb

Stay search and listings, mapped as a signed-out visitor. The one travel entry in
this atlas, and structurally unlike the commerce entries: a listing is a document
of named sections rather than a product with attributes, and search is paired
with a live map.

## Coverage

| View | Route | What it holds |
|---|---|---|
| `Home` | `/` | Expanded search form and vertical tabs |
| `SearchResults` | `/s/:location/homes` | Listing card grid beside a Google Map |
| `ListingDetail` | `/rooms/:listingId` | Sectioned listing page and booking panel |
| `CityLanding` | `/:citySlug/stays` | SEO carousels of listing cards |
| `NotFound` | `/404` | Error page, reached by redirect |

`SkipLinks`, `AppRoot`, `PageHeader`, `PageMain`, `PageFooter`, `SearchHeader`,
`HeaderProfileMenu`, `TabListWrapper`, `ContentScroller`, and `ModalContainer`
are global. All five views report `0 orphaned` coverage, with every selector
`sel-probe`d on 2026-08-07.

## What bites

**Four naming conventions, each covering a different part of the site.**
Kebab-case testids (`listing-card-title`, `little-search`) name interactive
components. UPPER_SNAKE testids (`LISTINGS_CAROUSEL`, `HEADER`, `BANNER`) name
sections on SEO landing pages. `data-section-id` (`TITLE_DEFAULT`,
`AMENITIES_DEFAULT`) names the structure of a listing page. Slash-namespaced
testids (`map/GoogleMap`, `map/ZoomInButton`) belong to the map. An agent
querying only `data-testid` never sees a listing page's outline at all.

**Section ids carry experiment variants.** `OVERVIEW_DEFAULT_V2` sits alongside
`TITLE_DEFAULT` on the same page, so the exact id depends on which variant was
served. Match on the prefix.

**Cards and map markers are different trees describing the same listings.** One
load had 28 cards and 20 markers, because the map only renders markers inside the
current viewport. Neither number is the result count.

**Prices are a function of the search, not the listing.** The dates and guest
count in the search state determine what a card shows, so a price is only
meaningful together with the query that produced it.

**The search control changes shape between routes.** The home page uses the
expanded `structured-search-input-*` form; every other route uses the collapsed
`little-search`. Learning one does not find the other.

**Testids can encode state.** `pdp-save-button-unsaved` becomes the saved variant
once toggled, so a selector for the unsaved form stops matching after a save.

**Class names are worthless here.** They are Linaria output, and 31 elements
carry `data-testid="linaria-injector"` as style injection points. Nothing on this
site should be selected by class.

## The 404 is the good example

An unmatched path redirects to `/404`, the title reads "404 Page Not Found -
Airbnb", and the body spells out the error code. Path, title, and text all agree,
so any one of the three detects it. The page also drops the React root entirely,
which makes the absence of `#react-application` a fourth independent signal.

Worth comparing against the other entries in this atlas, where Amazon's 404 has
no text at all, Vuori's has an empty document title, Nike's merchandises real
products, and eBay's says "We looked everywhere" without a code.

## Known gaps

Booking stops at the reserve button. Activating it starts a real reservation
flow on a live marketplace.

Accounts, wishlists, trips, and messaging are out of scope per
[docs/POLICY.md](../../docs/POLICY.md).

Experiences and Services are not mapped. Both normalize away from `/s/` to
different route shapes (`/s/Paris--France/experiences` becomes
`/ile-de-france/things-to-do`), and each deserves its own view rather than being
folded into the stay search.

`ListingDetail` is pinned to one listing, which happens to be a hotel and so
carries `PROPERTY_AVAILABLE_ROOMS`. A whole-home listing renders a different
subset of sections; the section vocabulary holds, the specific list does not.

## Screenshots

One screenshot, of signed-out search results. The listing detail page is
deliberately not pictured: it carries host names and guest reviews, and
[docs/POLICY.md](../../docs/POLICY.md) rules out personal data in screenshots.
The corpus records that page's structure without reproducing its content.
