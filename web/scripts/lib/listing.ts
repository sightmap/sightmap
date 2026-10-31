// Turns a scan report plus its review into the two files a listing is:
//
//   src/data/directory/<slug>.yaml               the listing
//   src/data/directory/scans/<slug>/<date>.json  the report it was reviewed against
//
// Two properties matter here and both come from the contract in
// src/data/directory/README.md:
//
//   Slugs are unique across the community atlas *and* the directory, because
//   both render at /atlas/<slug>. So the reserved set is the atlas's entry
//   slugs plus every listing already on disk — including ones loadDirectory
//   dropped as invalid, whose file still occupies the name.
//
//   Every step is idempotent. A rescan of a host that is already listed is not
//   a new listing: it keeps the slug, the date it was added, and everything a
//   maintainer edited by hand (labels, collections, the stored journey, and —
//   unless `replaceReview` says otherwise — the approved description and
//   category), adds a dated report next to the old ones, and bumps `updated`.
import fs from 'node:fs'
import path from 'node:path'
import type { DirectoryListing, ListingMeta, ListingTool, ListingType, ScanReport } from '../../src/types/directory'
import { loadAtlas } from './atlas'
import { assignLots, planCity } from './city'
import { ListingSchema, canonicalHost, issuesOf, listingToYaml, loadDirectory, slugFromHost, uniqueSlug } from './directory'
import type { Review } from './review'

export type SubmittedBy = 'owner' | 'nominator' | 'maintainer'

export interface CreateListingInput {
  /** `src/data/directory`. */
  dataDir: string
  /** `src/data/atlas` — read only, for the slugs the community entries hold. */
  atlasDir: string
  report: ScanReport
  review: Review
  type?: ListingType
  sightkick?: boolean
  submittedBy?: SubmittedBy
  /** YYYY-MM-DD written into `added`/`updated`. Defaults to the local date. */
  today?: string
  /** Overwrite a maintainer-edited description and category on a rescan. */
  replaceReview?: boolean
}

export interface CreatedListing {
  slug: string
  /** Absolute path of the YAML that was written. */
  listingPath: string
  /** Absolute path of the scan report that was written. */
  scanPath: string
  isRescan: boolean
}

export function todayString(d = new Date()): string {
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}

/**
 * A display name for a new listing. Page titles are usually
 * "Name — tagline" or "Name | Section"; the part before the first separator is
 * the name. Falls back to the host, which is always true and never wrong.
 */
export function listingName(report: ScanReport): string {
  const title = report.hints.title.replace(/\s+/g, ' ').trim()
  // A colon needs no leading space ("Attio: The CRM…"); the dash family does,
  // so a hyphenated name ("flatwrite-md") survives.
  const segments = title.split(/\s*:\s+|\s+[—–|·-]\s+/).map((x) => x.trim()).filter(Boolean)
  // "Tagline | Brand" puts the name last. The host's own label is the one
  // word we know belongs to the site, so a segment that carries it wins.
  const label = report.host.replace(/^www\./, '').split('.')[0].toLowerCase()
  const branded = segments.find((seg) => seg.toLowerCase().replace(/[^a-z0-9]/g, '').includes(label.replace(/[^a-z0-9]/g, '')))
  const head = (branded && branded.length <= 60 ? branded : segments[0]) ?? ''
  if (!head || head.length > 60) return report.host
  return head
}

/** Every slug that is already spoken for, valid listing or not. */
function takenSlugs(dataDir: string, listings: DirectoryListing[], reserved: string[]): Set<string> {
  const taken = new Set<string>(reserved)
  for (const l of listings) taken.add(l.slug)
  if (fs.existsSync(dataDir)) {
    for (const file of fs.readdirSync(dataDir)) {
      if (/\.ya?ml$/.test(file)) taken.add(file.replace(/\.ya?ml$/, ''))
    }
  }
  return taken
}

/**
 * The dated report path, with the `-2`, `-3` … suffix the schema allows when a
 * host is scanned more than once on the same day.
 */
export function nextScanPath(dataDir: string, slug: string, date: string): string {
  const dir = path.join(dataDir, 'scans', slug)
  let rel = `scans/${slug}/${date}.json`
  for (let i = 2; fs.existsSync(path.join(dir, path.basename(rel))); i++) {
    rel = `scans/${slug}/${date}-${i}.json`
  }
  return rel
}

/** The reviewed tool summary: the scan's tools, wearing the review's kinds. */
export function listingTools(report: ScanReport, review: Review): ListingTool[] {
  const kinds = new Map(review.tools.map((t) => [t.name, t.kind]))
  return report.tools.map((t) => ({
    name: t.name,
    kind: kinds.get(t.name) ?? t.risk,
    description: t.description.replace(/\s+/g, ' ').trim(),
    page: t.page,
  }))
}

export async function createListing(input: CreateListingInput): Promise<CreatedListing> {
  const { dataDir, atlasDir, report, review } = input
  const today = input.today ?? todayString()

  const atlas = await loadAtlas(atlasDir)
  const reserved = atlas.entries.map((e) => e.slug)
  const { listings } = loadDirectory(dataDir, reserved)

  // A rescan matches on the www-stripped host: a site that answers on both
  // `acme.com` and `www.acme.com` (or that started redirecting to one of them
  // between scans) is one listing, not two competing for the same slug.
  const existing = listings.find((l) => canonicalHost(l.host) === canonicalHost(report.host))
  const isRescan = Boolean(existing)
  const slug = existing ? existing.slug : uniqueSlug(slugFromHost(report.host), takenSlugs(dataDir, listings, reserved))

  // The report is filed under the day it was taken, not the day it is written:
  // `scans/<slug>/<date>.json` is the scan's own history.
  const scanDate = report.scannedAt.slice(0, 10)
  const scanRel = nextScanPath(dataDir, slug, scanDate)
  const scanPath = path.join(dataDir, scanRel)
  fs.mkdirSync(path.dirname(scanPath), { recursive: true })
  fs.writeFileSync(scanPath, `${JSON.stringify(report, null, 2)}\n`)

  // A maintainer approved the description and category by hand; a rescan is
  // new evidence about the tools, not a reason to overwrite their prose.
  const keepProse = existing && !input.replaceReview

  const meta: ListingMeta = {
    slug,
    name: existing?.name ?? listingName(report),
    url: report.finalUrl,
    host: report.host,
    description: keepProse ? existing!.description : review.description,
    category: keepProse ? existing!.category : review.category,
    type: input.type ?? existing?.type ?? review.type,
    built_with_sightkick: input.sightkick ?? existing?.built_with_sightkick ?? false,
    submitted_by: input.submittedBy ?? existing?.submitted_by ?? 'maintainer',
    labels: existing?.labels ?? [],
    collections: existing?.collections ?? [],
    lot: existing?.lot,
    added: existing?.added ?? today,
    updated: today,
    scan: scanRel,
    tools: listingTools(report, review),
    suggested_journeys: review.suggested_journeys,
    strengths: review.strengths,
    improvements: review.improvements,
  }
  if (existing?.journey) meta.journey = existing.journey
  // A new listing takes its lot now, with every other listing already in
  // place, and the YAML records it so the PR that adds the listing shows the
  // address and no later build can move it.
  if (meta.lot === undefined) {
    const others = listings.filter((l) => l.slug !== slug)
    const assigned = assignLots(planCity(), [...others, meta]).find((a) => a.slug === slug)
    if (assigned) meta.lot = assigned.lot
  }

  const parsed = ListingSchema.safeParse(meta)
  if (!parsed.success) {
    throw new Error(`refusing to write an invalid listing ${slug}:\n${issuesOf(parsed.error)}`)
  }

  const listingPath = path.join(dataDir, `${slug}.yaml`)
  fs.writeFileSync(listingPath, listingToYaml(parsed.data as ListingMeta))

  return { slug, listingPath, scanPath, isRescan }
}
