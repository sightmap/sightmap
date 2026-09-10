// Shape of one WebMCP directory listing as the app consumes it. A listing is
// the lightweight sibling of an atlas entry (src/types/atlas.ts): where an
// entry is a vendored sightmap corpus, a listing is a site that exposes
// callable WebMCP tools, plus the scan report that inspected them.
//
// Field names are snake_case on purpose, the same convention the atlas entry
// contract uses, so the YAML a maintainer edits and the JSON the site serves
// carry identical keys. The `_`-free derived fields at the bottom are computed
// by scripts/lib/directory.ts from the listing and its scan.

/** How a tool is classified. `classified by Atlas`, never by the site. */
export type ToolKind = 'read' | 'action' | 'sensitive'

/** Which WebMCP surface the scanner found the tools on. */
export type ScanSurface = 'native' | 'polyfilled' | 'declarative' | 'absent'

/**
 * Observable outcomes only. There is deliberately no `safe`, `trusted` or
 * `certified` here — a scan enumerates tools, it does not vouch for them.
 */
export type ScanStatus = 'tools-found' | 'api-empty' | 'api-absent' | 'blocked' | 'load-error' | 'needs-review'

export type ListingType = 'live' | 'demo'

export interface ScanTool {
  name: string
  description: string
  /**
   * The tool's input schema exactly as the page registered it. Optional
   * because a page may register none; JSON drops an undefined value, so a
   * required key here would make such a scan fail the generated manifest's
   * typecheck — a listing must never break the build.
   */
  inputSchema?: unknown
  /** First page (path) the tool was seen on. */
  page: string
  /** Every page (path) the tool was seen on. */
  pages: string[]
  impl: 'imperative' | 'declarative'
  /** Which object the registration landed on. */
  api: 'navigator' | 'document' | 'dom'
  risk: ToolKind
  riskReason: string
  warnings: string[]
}

export interface ScanPage {
  url: string
  path: string
  title: string
  status: number | null
  surface: ScanSurface
  tools: string[]
  error?: string
}

export interface ScanCheck {
  id: string
  label: string
  ok: boolean
  detail: string
}

export interface ScanCounts {
  tools: number
  pages: number
  read: number
  action: number
  sensitive: number
  described: number
  withInputSchema: number
  declarative: number
}

/** A form the scanner saw. Feeds the Sightkick starter, never rendered raw. */
export interface ScanFormHint {
  page: string
  action: string
  method: string
  fields: string[]
}

export interface ScanReport {
  version: 1
  url: string
  finalUrl: string
  host: string
  scannedAt: string
  scanner: { name: string; version: string; browser: string }
  surface: ScanSurface
  status: ScanStatus
  intent: string | null
  pages: ScanPage[]
  tools: ScanTool[]
  checks: ScanCheck[]
  counts: ScanCounts
  hints: { forms: ScanFormHint[]; links: string[]; title: string; description: string }
  notes: string[]
}

export interface ListingTool {
  name: string
  kind: ToolKind
  description: string
  page: string
}

export interface ListingJourney {
  intent: string
  outcome: 'passed' | 'failed'
  ran_at: string
  tools_called: string[]
  duration_ms: number
  notes: string
}

export interface SuggestedJourney {
  intent: string
  tools: string[]
}

export interface ListingMeta {
  slug: string
  name: string
  url: string
  host: string
  description: string
  category: string
  type: ListingType
  built_with_sightkick: boolean
  submitted_by: 'owner' | 'nominator' | 'maintainer'
  labels: string[]
  collections: string[]
  added: string
  updated: string
  /** Path of the latest scan report, relative to the directory data dir. */
  scan: string
  tools: ListingTool[]
  journey?: ListingJourney
  suggested_journeys: SuggestedJourney[]
  strengths: string[]
  improvements: string[]
}

export interface ScanDrift {
  /** Date of the scan this one is compared against. */
  since: string
  added: string[]
  removed: string[]
}

export interface DirectoryListing extends ListingMeta {
  report: ScanReport
  /** Every scan date on file for this slug, newest first. */
  scanDates: string[]
  drift: ScanDrift | null
  counts: ScanCounts
  scannedAt: string
  surface: ScanSurface
  status: ScanStatus
}

/** A Sightkick starter kit, generated from the scan by scripts/lib/sightkick-starter.ts. */
export interface SightkickStarter {
  kind: 'verify' | 'author'
  title: string
  intro: string
  code: string
  lang: 'sh' | 'yaml'
  prompt: string
}

/** What the app renders: a listing plus the build-time extras the page shows. */
export interface DirectoryListingView extends DirectoryListing {
  starter: SightkickStarter
}
