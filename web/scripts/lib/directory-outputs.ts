// The public JSON documents the Atlas publishes for its WebMCP listings:
// /atlas/directory.json, /atlas/stats.json, /atlas/sites/<slug>.json,
// /atlas/sites/<slug>/tools.json and /atlas/hosts/<host>.json.
//
// Everything here is a pure listing → document mapping. scripts/build-atlas.ts
// owns the filesystem (which files, wiped and rewritten on every build); this
// module owns the shapes, which is what an agent actually depends on — so the
// shapes are testable without a build, and a change to one is a change to a
// test rather than to a `JSON.stringify` buried in a loop.
//
// Keys are snake_case throughout, including the fields DirectoryListing
// carries in camelCase (`scannedAt`, `scanDates`). The YAML a maintainer
// writes and the JSON an agent fetches use one convention; the camelCase
// fields are an artifact of the scan report being written by a TypeScript
// tool, and they should not leak into the published contract.
import type {
  DirectoryListing,
  ListingJourney,
  ListingTool,
  ListingType,
  ScanCheck,
  ScanCounts,
  ScanDrift,
  ScanReport,
  ScanStatus,
  ScanSurface,
  ScanTool,
} from '../../src/types/directory'
import { SITE_URL } from './site'

export const DIRECTORY_SCHEMA_VERSION = 1

export interface ToolKindCounts {
  read: number
  action: number
  sensitive: number
}

/** The URLs every document repeats for one listing. */
export interface ListingUrls {
  html: string
  markdown: string
  json: string
  tools: string
  scan: string
  badge: string
}

export function listingUrls(slug: string): ListingUrls {
  return {
    html: `${SITE_URL}/atlas/${slug}`,
    markdown: `${SITE_URL}/atlas/${slug}.md`,
    json: `${SITE_URL}/atlas/sites/${slug}.json`,
    tools: `${SITE_URL}/atlas/sites/${slug}/tools.json`,
    scan: `${SITE_URL}/atlas/scans/${slug}.json`,
    badge: `${SITE_URL}/atlas/${slug}/badge.svg`,
  }
}

const kindCounts = (counts: ScanCounts): ToolKindCounts => ({
  read: counts.read,
  action: counts.action,
  sensitive: counts.sensitive,
})

/** ISO timestamp → the date an agent compares against `added` / `updated`. */
const dateOf = (iso: string): string => iso.slice(0, 10)

// ---------------------------------------------------------------- index

/** One line of /atlas/directory.json: enough to choose, not enough to call. */
export interface DirectoryIndexListing {
  slug: string
  name: string
  url: string
  host: string
  description: string
  category: string
  type: ListingType
  built_with_sightkick: boolean
  labels: string[]
  collections: string[]
  surface: ScanSurface
  status: ScanStatus
  tool_count: number
  tool_kinds: ToolKindCounts
  pages_scanned: number
  last_scanned: string
  added: string
  updated: string
  html: string
  json: string
  tools: string
  scan: string
  badge: string
}

export interface DirectoryIndexDocument {
  schema_version: number
  generated_at: string
  listings: DirectoryIndexListing[]
}

/**
 * The index carries no input schemas and no tool descriptions on purpose: an
 * agent resolving a hostname reads this file whole, and one listing's full
 * schemas can be larger than every summary in it put together. The per-tool
 * detail is one fetch away at `tools`.
 */
export function directoryIndexListing(listing: DirectoryListing): DirectoryIndexListing {
  const urls = listingUrls(listing.slug)
  return {
    slug: listing.slug,
    name: listing.name,
    url: listing.url,
    host: listing.host,
    description: listing.description,
    category: listing.category,
    type: listing.type,
    built_with_sightkick: listing.built_with_sightkick,
    labels: listing.labels,
    collections: listing.collections,
    surface: listing.surface,
    status: listing.status,
    tool_count: listing.counts.tools,
    tool_kinds: kindCounts(listing.counts),
    pages_scanned: listing.counts.pages,
    last_scanned: dateOf(listing.scannedAt),
    added: listing.added,
    updated: listing.updated,
    html: urls.html,
    json: urls.json,
    tools: urls.tools,
    scan: urls.scan,
    badge: urls.badge,
  }
}

export function directoryIndexDocument(
  listings: DirectoryListing[],
  generatedAt: string
): DirectoryIndexDocument {
  return {
    schema_version: DIRECTORY_SCHEMA_VERSION,
    generated_at: generatedAt,
    listings: listings.map(directoryIndexListing),
  }
}

// ---------------------------------------------------------------- stats

export interface DirectoryStatsDocument {
  schema_version: number
  generated_at: string
  listings: { live: number; demo: number; total: number }
  /** Community sightmap entries — the other half of /atlas, not listings. */
  community_maps: number
  tools: { total: number; read: number; action: number; sensitive: number; declarative: number }
  surfaces: Record<ScanSurface, number>
  categories: Record<string, number>
  built_with_sightkick: number
}

export interface StatsInput {
  generatedAt: string
  /** How many vendored community atlas entries ship alongside the listings. */
  communityMaps: number
}

export function directoryStatsDocument(
  listings: DirectoryListing[],
  { generatedAt, communityMaps }: StatsInput
): DirectoryStatsDocument {
  const surfaces: Record<ScanSurface, number> = { native: 0, polyfilled: 0, declarative: 0, absent: 0 }
  const categories: Record<string, number> = {}
  const tools = { total: 0, read: 0, action: 0, sensitive: 0, declarative: 0 }
  let live = 0
  let demo = 0
  let sightkick = 0

  for (const l of listings) {
    if (l.type === 'live') live++
    else demo++
    if (l.built_with_sightkick) sightkick++
    surfaces[l.surface]++
    categories[l.category] = (categories[l.category] ?? 0) + 1
    tools.total += l.counts.tools
    tools.read += l.counts.read
    tools.action += l.counts.action
    tools.sensitive += l.counts.sensitive
    tools.declarative += l.counts.declarative
  }

  return {
    schema_version: DIRECTORY_SCHEMA_VERSION,
    generated_at: generatedAt,
    listings: { live, demo, total: listings.length },
    community_maps: communityMaps,
    tools,
    surfaces,
    // Insertion order would follow the listings, which are sorted
    // newest-first; sorting the ids keeps the file stable across a rebuild
    // that only reordered them.
    categories: Object.fromEntries(Object.keys(categories).sort().map((k) => [k, categories[k]])),
    built_with_sightkick: sightkick,
  }
}

// ---------------------------------------------------------------- one site

/** /atlas/sites/<slug>.json — the listing, without the scan report inside it. */
export interface SiteDocument {
  schema_version: number
  slug: string
  name: string
  url: string
  host: string
  description: string
  category: string
  type: ListingType
  built_with_sightkick: boolean
  submitted_by: string
  labels: string[]
  collections: string[]
  added: string
  updated: string
  surface: ScanSurface
  status: ScanStatus
  scanned_at: string
  last_scanned: string
  counts: ScanCounts
  tool_kinds: ToolKindCounts
  tools: ListingTool[]
  checks: ScanCheck[]
  journey: ListingJourney | null
  suggested_journeys: DirectoryListing['suggested_journeys']
  strengths: string[]
  improvements: string[]
  drift: ScanDrift | null
  scan_dates: string[]
  links: ListingUrls & { scans: string[]; directory: string; stats: string }
}

/**
 * The report itself is deliberately not inlined: it is the largest thing a
 * listing owns and the least often needed, and it is already published
 * verbatim at `links.scan`. What comes across instead is the reviewed
 * summary — the counts, the checks a maintainer read, the journey they ran,
 * and what changed since the scan before.
 */
export function siteDocument(listing: DirectoryListing): SiteDocument {
  const urls = listingUrls(listing.slug)
  return {
    schema_version: DIRECTORY_SCHEMA_VERSION,
    slug: listing.slug,
    name: listing.name,
    url: listing.url,
    host: listing.host,
    description: listing.description,
    category: listing.category,
    type: listing.type,
    built_with_sightkick: listing.built_with_sightkick,
    submitted_by: listing.submitted_by,
    labels: listing.labels,
    collections: listing.collections,
    added: listing.added,
    updated: listing.updated,
    surface: listing.surface,
    status: listing.status,
    scanned_at: listing.scannedAt,
    last_scanned: dateOf(listing.scannedAt),
    counts: listing.counts,
    tool_kinds: kindCounts(listing.counts),
    tools: listing.tools,
    checks: listing.report.checks,
    journey: listing.journey ?? null,
    suggested_journeys: listing.suggested_journeys,
    strengths: listing.strengths,
    improvements: listing.improvements,
    drift: listing.drift,
    scan_dates: listing.scanDates,
    links: {
      ...urls,
      scans: listing.scanDates.map((d) => `${SITE_URL}/atlas/scans/${listing.slug}/${d}.json`),
      directory: `${SITE_URL}/atlas/directory.json`,
      stats: `${SITE_URL}/atlas/stats.json`,
    },
  }
}

// ---------------------------------------------------------------- tools

/** /atlas/sites/<slug>/tools.json — every tool with its full input schema. */
export interface ToolsDocument {
  slug: string
  scanned_at: string
  tools: ScanTool[]
}

/**
 * Straight off the scan, schemas and all: this is the file an agent reads
 * when it has decided to use the site and needs to know what it can call.
 * The listing's own `tools[]` is the reviewed summary and can be shorter —
 * a maintainer drops a tool from the listing without the scan forgetting it.
 */
export function toolsDocument(listing: DirectoryListing): ToolsDocument {
  return {
    slug: listing.slug,
    scanned_at: listing.scannedAt,
    tools: listing.report.tools,
  }
}

// ---------------------------------------------------------------- hosts

/** /atlas/hosts/<host>.json, served at /api/atlas/lookup/<host>. */
export interface HostDocument {
  slug: string
  url: string
  tool_count: number
  last_scanned: string
  html: string
  json: string
}

export interface HostFile {
  /** File name stem: the host, already checked to be path-safe. */
  host: string
  document: HostDocument
}

// A host reaches the filesystem as a file name, so it is checked rather than
// trusted: letters, digits, dots and hyphens only, no leading dot, no `..`.
// A listing whose host does not pass simply publishes no lookup file — the
// rest of it still ships, the same per-listing contract loadDirectory uses.
const SAFE_HOST = /^[a-z0-9]([a-z0-9.-]*[a-z0-9])?$/

export function isSafeHost(host: string): boolean {
  return SAFE_HOST.test(host) && !host.includes('..')
}

/**
 * The lookup documents for one listing.
 *
 * An agent arrives holding whatever hostname the address bar showed, which is
 * `www.example.com` about as often as `example.com`. Publishing both spellings
 * makes the lookup a static file fetch instead of a normalisation the caller
 * has to know to perform.
 */
export function hostDocuments(listing: DirectoryListing): HostFile[] {
  const urls = listingUrls(listing.slug)
  const document: HostDocument = {
    slug: listing.slug,
    url: listing.url,
    tool_count: listing.counts.tools,
    last_scanned: dateOf(listing.scannedAt),
    html: urls.html,
    json: urls.json,
  }

  const host = listing.host.trim().toLowerCase()
  const bare = host.replace(/^www\./, '')
  const hosts = [bare, `www.${bare}`].filter((h, i, all) => all.indexOf(h) === i && isSafeHost(h))
  return hosts.map((h) => ({ host: h, document }))
}

// ---------------------------------------------------------------- manifest

// The CLI transcript the scanner records is useful in the published scan JSON
// (it is how someone reproduces the run) and dead weight in the app bundle,
// where nothing renders it. src/generated/atlas-manifest.ts is imported by the
// client, so the transcript lines are dropped from that copy only.
const CLI_NOTE = /^\$ /

/** The report as the manifest carries it: same object, no `$ ` transcript. */
export function reportWithoutTranscript(report: ScanReport): ScanReport {
  const notes = report.notes.filter((n) => !CLI_NOTE.test(n))
  return notes.length === report.notes.length ? report : { ...report, notes }
}
