// The single reader for src/data/directory — the WebMCP listings the Atlas
// publishes alongside the vendored community sightmaps. Same contract as
// scripts/lib/atlas.ts: validation is per listing and non-fatal, so a listing
// with a broken YAML file or a missing scan is dropped from the gallery with a
// loud build-log warning while the rest of the site ships.
//
// A listing is two files:
//
//   src/data/directory/<slug>.yaml               the listing (maintainer-edited)
//   src/data/directory/scans/<slug>/<date>.json  scan reports, one per scan
//
// The YAML names the scan it was reviewed against (`scan:`); older reports are
// kept so a rescan can be diffed against the one before it (`drift`).
import fs from 'node:fs'
import path from 'node:path'
import YAML from 'yaml'
import { z } from 'zod'
import type { DirectoryListing, ListingMeta, ScanDrift, ScanReport } from '../../src/types/directory'

const DATE = /^\d{4}-\d{2}-\d{2}$/
const SLUG = /^[a-z0-9]+(-[a-z0-9]+)*$/

export const ToolKindSchema = z.enum(['read', 'action', 'sensitive'])
export const SurfaceSchema = z.enum(['native', 'polyfilled', 'declarative', 'absent'])
export const StatusSchema = z.enum(['tools-found', 'api-empty', 'api-absent', 'blocked', 'load-error', 'needs-review'])

const ScanToolSchema = z.object({
  name: z.string(),
  description: z.string().default(''),
  inputSchema: z.unknown().optional(),
  page: z.string().min(1),
  pages: z.array(z.string().min(1)).default([]),
  impl: z.enum(['imperative', 'declarative']).default('imperative'),
  api: z.enum(['navigator', 'document', 'dom']).default('navigator'),
  risk: ToolKindSchema,
  riskReason: z.string().default(''),
  warnings: z.array(z.string()).default([]),
})

const ScanPageSchema = z.object({
  url: z.string().min(1),
  path: z.string().min(1),
  title: z.string().default(''),
  status: z.number().int().nullable().default(null),
  surface: SurfaceSchema,
  tools: z.array(z.string()).default([]),
  error: z.string().optional(),
})

const ScanCheckSchema = z.object({
  id: z.string().min(1),
  label: z.string().min(1),
  ok: z.boolean(),
  detail: z.string().default(''),
})

const CountsSchema = z.object({
  tools: z.number().int().nonnegative(),
  pages: z.number().int().nonnegative(),
  read: z.number().int().nonnegative(),
  action: z.number().int().nonnegative(),
  sensitive: z.number().int().nonnegative(),
  described: z.number().int().nonnegative(),
  withInputSchema: z.number().int().nonnegative(),
  declarative: z.number().int().nonnegative().default(0),
})

export const ScanReportSchema = z.object({
  version: z.literal(1),
  url: z.string().url(),
  finalUrl: z.string().url(),
  host: z.string().min(1),
  scannedAt: z.string().min(1),
  scanner: z.object({ name: z.string(), version: z.string(), browser: z.string() }),
  surface: SurfaceSchema,
  status: StatusSchema,
  intent: z.string().nullable().default(null),
  pages: z.array(ScanPageSchema).default([]),
  tools: z.array(ScanToolSchema).default([]),
  checks: z.array(ScanCheckSchema).default([]),
  counts: CountsSchema,
  hints: z
    .object({
      forms: z
        .array(
          z.object({
            page: z.string(),
            action: z.string().default(''),
            method: z.string().default('get'),
            fields: z.array(z.string()).default([]),
          })
        )
        .default([]),
      links: z.array(z.string()).default([]),
      title: z.string().default(''),
      description: z.string().default(''),
    })
    .default({ forms: [], links: [], title: '', description: '' }),
  notes: z.array(z.string()).default([]),
})

const ListingToolSchema = z.object({
  name: z.string().min(1),
  kind: ToolKindSchema,
  description: z.string().default(''),
  page: z.string().default('/'),
})

const JourneySchema = z.object({
  intent: z.string().min(1),
  outcome: z.enum(['passed', 'failed']),
  ran_at: z.string().regex(DATE, 'must be YYYY-MM-DD'),
  tools_called: z.array(z.string()).default([]),
  duration_ms: z.number().int().nonnegative().default(0),
  notes: z.string().default(''),
})

export const ListingSchema = z.object({
  slug: z.string().regex(SLUG, 'must be lowercase kebab-case'),
  name: z.string().min(1),
  url: z.string().url(),
  host: z.string().min(1),
  description: z.string().min(1),
  category: z.string().regex(/^[a-z][a-z0-9-]*$/, 'must be a lowercase category id'),
  type: z.enum(['live', 'demo']),
  built_with_sightkick: z.boolean().default(false),
  submitted_by: z.enum(['owner', 'nominator', 'maintainer']).default('maintainer'),
  labels: z.array(z.enum(['promising', 'verified', 'featured'])).default([]),
  collections: z.array(z.string().regex(/^[a-z][a-z0-9-]*$/)).default([]),
  added: z.string().regex(DATE, 'must be YYYY-MM-DD'),
  updated: z.string().regex(DATE, 'must be YYYY-MM-DD'),
  scan: z.string().regex(/^scans\/[a-z0-9-]+\/\d{4}-\d{2}-\d{2}(-\d+)?\.json$/, 'must be scans/<slug>/<date>.json'),
  tools: z.array(ListingToolSchema).default([]),
  journey: JourneySchema.optional(),
  suggested_journeys: z
    .array(z.object({ intent: z.string().min(1), tools: z.array(z.string()).default([]) }))
    .default([]),
  strengths: z.array(z.string()).default([]),
  improvements: z.array(z.string()).default([]),
})

export interface LoadedDirectory {
  listings: DirectoryListing[]
  /** Slugs dropped by validation, so a caller can report them. */
  skipped: string[]
}

/** Zod issues as an indented bullet list, for a "refusing to …" error message. */
export function issuesOf(error: z.ZodError): string {
  return error.issues.map((i) => `    - ${i.path.join('.') || '(root)'}: ${i.message}`).join('\n')
}

/** `scans/<slug>/2026-09-09.json` → `2026-09-09`. */
export function scanDateOf(file: string): string {
  return path.basename(file).replace(/\.json$/, '')
}

/**
 * Every scan report on file for a slug, newest first. Only files named like a
 * date are considered, so a stray `review.json` or `.md` next to them is not
 * mistaken for a scan.
 */
export function scanFilesFor(dataDir: string, slug: string): string[] {
  const dir = path.join(dataDir, 'scans', slug)
  if (!fs.existsSync(dir)) return []
  return fs
    .readdirSync(dir)
    .filter((f) => /^\d{4}-\d{2}-\d{2}(-\d+)?\.json$/.test(f))
    .sort()
    .reverse()
    .map((f) => `scans/${slug}/${f}`)
}

export function readScan(dataDir: string, rel: string): ScanReport {
  const raw = JSON.parse(fs.readFileSync(path.join(dataDir, rel), 'utf-8'))
  const parsed = ScanReportSchema.safeParse(raw)
  if (!parsed.success) throw new Error(`invalid scan ${rel}:\n${issuesOf(parsed.error)}`)
  return parsed.data as ScanReport
}

/** Tool names added and removed between two scans. */
export function diffScans(older: ScanReport, newer: ScanReport): ScanDrift {
  const before = new Set(older.tools.map((t) => t.name))
  const after = new Set(newer.tools.map((t) => t.name))
  return {
    since: older.scannedAt.slice(0, 10),
    added: [...after].filter((n) => !before.has(n)).sort(),
    removed: [...before].filter((n) => !after.has(n)).sort(),
  }
}

/**
 * Loads every listing. `reserved` is the set of slugs already taken by the
 * community atlas: both kinds of page render at /atlas/<slug>, so a collision
 * would have two entries fighting over one file.
 */
export function loadDirectory(dataDir: string, reserved: Iterable<string> = []): LoadedDirectory {
  const listings: DirectoryListing[] = []
  const skipped: string[] = []
  const seen = new Set(reserved)

  if (!fs.existsSync(dataDir)) return { listings, skipped }

  const files = fs
    .readdirSync(dataDir)
    .filter((f) => /\.ya?ml$/.test(f))
    .sort()

  for (const file of files) {
    const label = file.replace(/\.ya?ml$/, '')
    let raw: unknown
    try {
      raw = YAML.parse(fs.readFileSync(path.join(dataDir, file), 'utf-8'))
    } catch (err) {
      console.warn(`  ! skipping directory listing ${label}: ${err instanceof Error ? err.message : err}`)
      skipped.push(label)
      continue
    }

    const parsed = ListingSchema.safeParse(raw)
    if (!parsed.success) {
      console.warn(`  ! skipping directory listing ${label}:\n${issuesOf(parsed.error)}`)
      skipped.push(label)
      continue
    }
    const meta = parsed.data as ListingMeta

    if (meta.slug !== label) {
      console.warn(`  ! skipping directory listing ${label}: file name does not match slug "${meta.slug}"`)
      skipped.push(label)
      continue
    }
    if (seen.has(meta.slug)) {
      console.warn(`  ! skipping directory listing ${meta.slug}: slug already used by another atlas entry`)
      skipped.push(meta.slug)
      continue
    }

    let report: ScanReport
    try {
      report = readScan(dataDir, meta.scan)
    } catch (err) {
      console.warn(`  ! skipping directory listing ${meta.slug}: ${err instanceof Error ? err.message : err}`)
      skipped.push(meta.slug)
      continue
    }

    // A listed tool the scan never saw is a stale listing, not a scan bug —
    // warn, keep the listing, and let the page say so through the drift block.
    const scanned = new Set(report.tools.map((t) => t.name))
    for (const t of meta.tools) {
      if (!scanned.has(t.name)) {
        console.warn(`  ! directory listing ${meta.slug}: tool "${t.name}" is not in ${meta.scan}`)
      }
    }

    const scanFiles = scanFilesFor(dataDir, meta.slug)
    const scanDates = scanFiles.map(scanDateOf)
    let drift: ScanDrift | null = null
    const idx = scanFiles.indexOf(meta.scan)
    const previous = idx >= 0 ? scanFiles[idx + 1] : undefined
    if (previous) {
      try {
        drift = diffScans(readScan(dataDir, previous), report)
      } catch (err) {
        console.warn(`  ! directory listing ${meta.slug}: cannot diff against ${previous}: ${err instanceof Error ? err.message : err}`)
      }
    }

    seen.add(meta.slug)
    listings.push({
      ...meta,
      report,
      scanDates,
      drift,
      counts: report.counts,
      scannedAt: report.scannedAt,
      surface: report.surface,
      status: report.status,
    })
  }

  listings.sort((a, b) => (a.updated === b.updated ? a.slug.localeCompare(b.slug) : b.updated.localeCompare(a.updated)))
  return { listings, skipped }
}

/** Every distinct category across the given listings, alphabetical. */
export function directoryCategories(listings: DirectoryListing[]): string[] {
  return [...new Set(listings.map((l) => l.category))].sort()
}

/** Turns a host into a candidate slug: `www.example.co.uk` → `example-co-uk`. */
export function slugFromHost(host: string): string {
  return host
    .toLowerCase()
    .replace(/^www\./, '')
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
}

/** First free slug given the ones already taken: `example`, `example-2`, … */
export function uniqueSlug(base: string, taken: Set<string>): string {
  if (!taken.has(base)) return base
  for (let i = 2; ; i++) {
    const candidate = `${base}-${i}`
    if (!taken.has(candidate)) return candidate
  }
}

/** Serialises a listing the way the intake script and a maintainer write it. */
export function listingToYaml(meta: ListingMeta): string {
  const ordered: Record<string, unknown> = {}
  for (const key of Object.keys(ListingSchema.shape)) {
    const v = (meta as unknown as Record<string, unknown>)[key]
    if (v !== undefined) ordered[key] = v
  }
  return YAML.stringify(ordered, { lineWidth: 100 })
}
