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
| `<slug>/.sightmap/*` | the corpus files `index.json` lists under `files[]`, copied from atlas `entries/<slug>/.sightmap/` |

`scripts/build-atlas.ts` packs each corpus into `public/atlas/<slug>.tar.gz`,
which is what `sightmap atlas add <slug>` downloads. An entry whose corpus is
missing still renders; the install command on its page 404s, and the build logs
a warning naming the slug.

**Why vendored rather than fetched.** The site build stays fully local: no
network calls, so a malformed community merge can never break a sightmap.org
deploy, and a takedown propagates by rebuild rather than waiting on a CDN. The
tarball is served from here for the same reason. Pointing installs at
raw.githubusercontent would leave a removed entry installable while the gallery
correctly stops showing it.

The first entry was vendored by hand to unblock gallery development before the
publish workflow merged; the automation writes exactly this shape.
