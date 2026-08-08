---
name: Sightmap
slug: sightmap-org
site_url: https://sightmap.org/
domains: [sightmap.org]
description: Marketing site and blog for the Sightmap spec — landing page sections, blog index, and post pages.
categories: [docs, devtools]
author: chiplay
created: 2026-08-06
updated: 2026-08-06
last_verified: 2026-08-06
cli_version: 0.17.0
spec_version: 1
method: browser
auth: none
---

# sightmap.org

The project's own marketing site, mapped from the outside with no source access,
the same way any atlas entry is made.

## Coverage

Four routes, which is the whole public surface:

| View | Route | What it holds |
|---|---|---|
| `Home` | `/` | Hero, pitch, spec and memory explainers, install section |
| `BlogIndex` | `/blog` | One `BlogCard` per post |
| `BlogPost` | `/blog/:slug` | Title, publication date, prose, fenced code blocks |
| `NotFound` | `/**` | Catch-all 404, reached by any unmatched path |

`Navigation` and `Footer` are global components, present on every route. Coverage
is `0 orphaned` on all four, with every selector `sel-probe`d on 2026-08-06.

The quality bar asks for five views or a justification. Four is the entire site.
The nav's Spec, Memory, and Get started links are in-page anchors (`/#spec`,
`/#memory`, `/#start`) rather than routes, so treating them as views would invent
structure the site doesn't have. Everything else linked from the page leaves the
domain, to docs.sightmap.org, github.com, or npmjs.com.

## Quirks

`data-component` values carry a per-build suffix. The DOM ships
`data-component="Hero-a1b2c3"`, so selectors have to match with `^=`. An exact
`=` match works until the next deploy, then stops silently.

Trailing slashes are normalized server-side. Links point at `/blog`, but the
served page reports `location.pathname === "/blog/"`, so code comparing a clicked
href against the resulting path will disagree with itself.

The install command sits inside a link rather than behind a copy button, so
activating it navigates to npm. Read the text through `InstallCommand.command`.

Blog cards are anchors in their entirety. The whole card is the link, with no
inner "read more" to target.

Nothing on the site fetches data at runtime. It is statically prerendered, and
the only non-asset traffic is the Fullstory analytics snippet, captured as the
two global requests.

An unmatched path renders instead of erroring: the server returns the SPA shell
and the client renders `NotFound`. An HTTP status will not tell an agent it took
a wrong turn, so check for the component.

## Known gaps

- `BlogPost` was verified against the one post published at mapping time,
  `/blog/sightmap`. The route pattern is general, but the component set covers
  only what that post uses. A future post containing images or tables would need
  a re-map.
- Requests cover the analytics calls only, recorded from `performance` resource
  timings rather than a network trace. The header lists name headers worth
  watching and stop short of full payload shapes.
- No `snapshots/` are committed, so `coverage --trace` cannot re-check this entry
  offline. Re-verification needs a live browser.
