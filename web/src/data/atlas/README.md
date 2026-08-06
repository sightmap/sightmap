# Vendored atlas data

Do not hand-edit. This directory is the sightmap.org build's only source of
atlas content, and it is written by automation in the
[sightmap/atlas](https://github.com/sightmap/atlas) repo: its `publish.yml`
workflow regenerates `index.json` on every merge to `main` and opens a rolling
bot PR that copies the result here.

| File | Origin |
|---|---|
| `index.json` | `gen-index.mjs` output — schema in atlas `docs/SPEC.md` |
| `screenshots/<slug>/*` | copied from atlas `entries/<slug>/screenshots/` |
| `<slug>.md` | entry README (front matter + prose), for the machine-twin route |

**Why vendored rather than fetched.** The site build stays fully local: no
network calls, so a malformed community merge can never break a sightmap.org
deploy, and a takedown propagates by rebuild rather than waiting on a CDN.

The first entry was vendored by hand to unblock gallery development before the
publish workflow merged; the automation writes exactly this shape.
