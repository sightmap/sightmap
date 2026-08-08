---
name: Pinterest
slug: pinterest
site_url: https://www.pinterest.com/
domains: [pinterest.com, www.pinterest.com]
description: Pinterest's pin detail page, mapped signed in — closeup image, description, action bar, and the related-pin grid.
categories: [social]
author: chiplay
created: 2026-08-08
updated: 2026-08-08
last_verified: 2026-08-08
cli_version: 0.19.0
spec_version: 1
method: browser
auth: personal-account
---

# Pinterest

One view: a pin detail page. 19 components and 6 requests, every selector
counted against the live page.

## Why this is `auth: personal-account`

The related grid below a pin is assembled for the account, and the header
carries the account's avatar and notification badge. Both are mapped as
structure. No pin content, board, follow, or identity ships — the entry records
that the containers exist and what shape they have.

Only the author can re-verify this entry. `last_verified` means the author
re-checked with their own account; CI cannot, and neither can a reviewer.

There are no screenshots, because a pin page is other people's photographs and
the account's own recommendations end to end.

## What bites

**There is no `h1` on this page.** Not an empty one — none at all. Heading-based
route detection cannot work here. Test for `[data-test-id="pdp-container"]`.

**The 22 elements with `data-test-id="pin"` are not the pin you are looking
at.** They are the related grid. The pin itself lives in the closeup container
and carries no `pin` attribute, so the obvious selector returns 22 wrong answers
and never the right one.

**The pin's own data never crosses the network.** It is rendered into the
document; only the related grid is fetched afterwards. Waiting on a request
before reading the pin waits for something that will not arrive.

**There is an explicit settle marker.** `[data-test-id="closeup-data-loaded"]`
appears once the pin has rendered. Poll for it rather than guessing a delay.

**Related cards are not uniform.** Of 22 observed, 21 carried an image, two were
video, and only 17 had a footer or a more-actions button. Counts across the grid
do not line up with the card count.

**44 images carry `pin-missing-alt-text`**, so alt text is not a dependable
description source on this page.

**Every backend call is `/resource/:ResourceName/:verb/`** where the verb is
`get`, `create` or `update`, and the arguments ride in a `data=` query parameter
holding URL-encoded JSON. There is no REST path to read, and the related grid
pages through a `bookmarks` cursor inside that JSON.

## Coverage

18 selectors counted on the route that declares them, all matching. One was
mis-scoped on the first pass: the navigation icons sit in their own `nav`
outside the header container, so scoping them under the header matched nothing.

`sightmap sel-probe` cannot attach to the browser this was authored in, so
matches were counted in-page instead.
