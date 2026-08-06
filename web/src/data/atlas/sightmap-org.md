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

The project's own marketing site, mapped from the outside with no source access —
the same way any atlas entry is made.

## Coverage

Four routes, which is the whole public surface:

| View | Route | What it holds |
|---|---|---|
| `Home` | `/` | Hero, pitch, spec and memory explainers, install section |
| `BlogIndex` | `/blog` | One `BlogCard` per post |
| `BlogPost` | `/blog/:slug` | Title, publication date, prose, fenced code blocks |
| `NotFound` | `/**` | Catch-all 404, reached by any unmatched path |

`Navigation` and `Footer` are global components — they appear on every route.
Coverage is `0 orphaned` on all four, with every selector `sel-probe`d on
2026-08-06.

**Why fewer than five views.** The quality bar asks for five or a justification.
Four is the entire site: the nav's Spec, Memory, and Get started links are
in-page anchors (`/#spec`, `/#memory`, `/#start`), not routes, so treating them
as views would invent structure that isn't there. Everything else linked from
the page leaves the domain (docs.sightmap.org, github.com, npmjs.com).

## What's worth knowing

The notes are the point of a sightmap, so the interesting ones up front:

- **`data-component` values are build-suffixed.** The DOM ships
  `data-component="Hero-a1b2c3"`, so every selector matches with `^=`. An exact
  `=` match works until the next deploy and then silently stops.
- **Trailing slashes are normalized server-side.** Links point at `/blog`, but
  the served page reports `location.pathname === "/blog/"`. Code that compares a
  clicked href to the resulting path will disagree with itself.
- **The install command is a link, not a copy button.** Activating it navigates
  to npm. Read the text through `InstallCommand.command` instead.
- **Blog cards are anchors.** The whole card is the link; there is no inner
  "read more" to target.
- **No application API.** The site is statically prerendered and fetches nothing
  at runtime — the only non-asset traffic is the Fullstory analytics snippet,
  captured as the two global requests.
- **A bad path renders, it doesn't error.** Unmatched paths get the SPA shell and
  a client-rendered `NotFound`, so an HTTP status won't tell an agent it took a
  wrong turn — check for the component.

## Known gaps

- `BlogPost` was verified against the single post published at mapping time
  (`/blog/sightmap`). The route pattern is general; the component set is what
  that post exercises, so a future post using elements it doesn't contain (say,
  images or tables) would need a re-map.
- Requests are the analytics calls only, recorded from `performance` resource
  timings rather than a network trace. Header lists name the headers worth
  watching, not full payload shapes.
- No `snapshots/` are committed, so `coverage --trace` can't re-check this entry
  offline; re-verification needs a live browser.
