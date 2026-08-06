# Phase 4 handoff — the atlas gallery on sightmap.org

Build `/atlas` and `/atlas/:slug` in this repo's `web/` app. Everything you need
is committed on this branch; nothing depends on an unmerged PR.

## Background in one paragraph

The [Sightmap Atlas](https://github.com/sightmap/atlas) is a community
collection of sightmaps of real websites — structured maps of views,
components, properties, and requests that an agent produced by browsing the
live site with no source access. This phase gives it a public face: a card grid
and a detail page, served from sightmap.org, built from data vendored into
`web/src/data/atlas/` by atlas CI. There is exactly one entry today
(`sightmap-org`); the design must not assume one, and must not assume hundreds.

## What already exists

- `web/src/data/atlas/index.json` — real generated data, one entry. Schema is
  in the atlas repo's `docs/SPEC.md`; treat every field name as a contract.
- `web/src/data/atlas/screenshots/sightmap-org/*.webp` — 1600px wide, ~60 KB.
- `web/src/data/atlas/sightmap-org.md` — the entry README: YAML front matter
  then prose. Source for the detail page's body and the `.md` machine twin.
- `web/src/data/atlas/README.md` — why the data is vendored, not fetched.

## Ground rules

1. **The build stays fully local.** No network fetch at build or run time. A
   broken community merge must never break a sightmap.org deploy.
2. **Match the existing site.** Reuse its design tokens, typography, and
   layout primitives; look at `web/src/pages/BlogIndex.tsx` and `BlogPost.tsx`
   first — they are the closest precedent for list-plus-detail and they already
   solve prerendering, meta tags, and OG images here.
3. **Prerendering is hand-rolled, not a framework feature.** Routes are
   declared in `web/src/App.tsx` and *again* in `web/scripts/prerender.tsx`,
   which writes one static file per URL. A route that renders in dev but is
   missing from the prerender script ships as a client-only page — check both.
4. **Degrade honestly.** Fields that are optional in the schema (`commit`, and
   entries with no screenshots — allowed by policy) must render without a hole.

## Tasks

### P4.1 — `/atlas` card grid

Card shows: screenshot (first one; a typographic fallback when absent), site
favicon, name, description, method pill, `N views · M components · K requests`
from `stats`, author, and `updated`. Client-side category filter and text
search, filter state reflected in the URL query string so a filtered view is
linkable. No JS-heavy dependency for this — the existing site ships very little.

### P4.2 — `/atlas/:slug` detail page

Modeled on a spec sheet, in this order: domain eyebrow (favicon + domain,
linking out), display title, an install block showing `sightmap add <slug>`
with a copy button, the description, a screenshot gallery with `FIG. 01`-style
captions, a per-view table (view, route, components, requests) from `per_view`,
and the entry README's prose body rendered below. A monospace metadata sidebar
carries CAPTURE METHOD, AUTH, LAST VERIFIED, CLI VERSION, SPEC VERSION, AUTHOR.

`sightmap add` ships in sightmap/sightmap#168, unmerged at handoff time. Render
the command anyway — it is the page's call to action and the PR is in review.

### P4.3 — machine twins

The atlas is for agents, so the pages need machine-readable counterparts:
`/atlas/index.json` served verbatim, `/atlas/<slug>.md` (front matter + body),
and one line per entry appended to the site's existing `llms.txt`. Follow
whatever the blog already does for static file emission.

### P4.4 — verification

`pnpm build` green; every route in `index.json` present in `dist/` as a real
static file with correct meta and OG tags; no network calls in the build; both
light and dark themes; mobile width without horizontal scroll. Add per-entry OG
images if the existing OG pipeline (`og/render.mjs`) makes it straightforward —
skip and say so if it does not.

## Definition of done

A PR against `main` with all four tasks, real `pnpm build` output in the
description, and screenshots of both pages. Note anything you deliberately left
undone and why.
