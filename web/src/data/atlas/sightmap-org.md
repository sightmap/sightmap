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

The project's own marketing site, mapped from the outside with no source access.

## Coverage

Four routes, which is the whole public surface:

| View | Route | What it holds |
|---|---|---|
| `Home` | `/` | Hero, pitch, spec and memory explainers, install section |
| `BlogIndex` | `/blog` | One `BlogCard` per post |
| `BlogPost` | `/blog/:slug` | Title, publication date, prose, fenced code blocks |
| `NotFound` | `/**` | Catch-all 404, reached by any unmatched path |

`Navigation` and `Footer` appear on every route. Coverage is `0 orphaned` on all
four, with every selector `sel-probe`d on 2026-08-06.

The nav's Spec, Memory, and Get started links are in-page anchors (`/#spec`,
`/#memory`, `/#start`), not routes. Everything else it links to leaves the
domain: docs.sightmap.org, github.com, npmjs.com.

## Notes

- **`data-component` values are build-suffixed.** The DOM ships
  `data-component="Hero-a1b2c3"`, so selectors need `^=`. An exact `=` match
  works until the next deploy, then stops matching with no error.
- **Trailing slashes are normalized server-side.** Links point at `/blog`; the
  served page reports `location.pathname === "/blog/"`. Compare normalized
  paths, not the raw href.
- **The install command is a link, not a copy button.** Activating it navigates
  to npm. Read the text from `InstallCommand.command` instead.
- **Blog cards are anchors.** The whole card is the link; there is no inner
  "read more" to target.
- **No application API.** The site is statically prerendered and fetches nothing
  at runtime. The only non-asset traffic is the Fullstory snippet, recorded as
  the two global requests.
- **A bad path renders, it doesn't error.** Unmatched paths get the SPA shell and
  a client-rendered `NotFound`, and the status stays 200. Check for the
  component, not the status code.

## Known gaps

- `BlogPost` was verified against the one post published at mapping time
  (`/blog/sightmap`). The route pattern is general, but the component set is
  only what that post uses; a post with images or tables would need a re-map.
- Requests cover the analytics calls only, recorded from `performance` resource
  timings rather than a network trace. Header lists name the headers worth
  watching, not full payload shapes.
- No `snapshots/` are committed, so `coverage --trace` cannot re-check this entry
  offline. Re-verification needs a live browser.
