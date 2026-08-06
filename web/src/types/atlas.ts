// Shape of one atlas entry as the app consumes it: every field name here is
// the one the atlas repo's `docs/SPEC.md` defines (see
// src/data/atlas/README.md), plus the two `_`-free derived fields at the
// bottom that scripts/build-atlas.ts computes. Keeping the vendored names
// verbatim — snake_case and all — means a schema addition upstream is a
// one-line change here rather than a rename hunt.

export interface AtlasPerView {
  name: string
  route: string
  components: number
  requests: number
}

export interface AtlasStats {
  views: number
  components: number
  requests: number
  properties: number
  memory: number
}

export interface AtlasEntryMeta {
  slug: string
  name: string
  site_url: string
  domains: string[]
  description: string
  categories: string[]
  author: string
  created: string
  updated: string
  last_verified: string
  cli_version: string
  spec_version: number
  method: string
  auth: string
  stats: AtlasStats
  per_view: AtlasPerView[]
  /**
   * Entry-relative paths exactly as index.json lists them
   * (`screenshots/01-home.webp`). Optional in the schema: an entry may ship
   * none. Use `screenshotUrls` to actually render — these are kept verbatim so
   * the machine twin and the contract stay honest.
   */
  screenshots: string[]
  files: string[]
  /** Optional in the schema — an entry mapped from a live site may have none. */
  commit?: string
}

export interface AtlasEntry extends AtlasEntryMeta {
  /**
   * Servable URLs under /atlas/screenshots/<slug>/, one per entry in
   * `screenshots` that was actually vendored. A path listed in index.json with
   * no file on disk is dropped here rather than rendered as a broken image.
   */
  screenshotUrls: string[]
  /** The entry README's prose body (front matter stripped), rendered to HTML. */
  bodyHtml: string
}
