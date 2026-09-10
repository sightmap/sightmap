// Adds what a scan saw to the unlisted card at /try/<host>.
//
//   pnpm atlas:card --host example.org --scan src/data/directory/scans/<slug>/<date>.json
//
// Run straight after `pnpm atlas:intake`, on both runner paths. It only ever
// *updates*: a card exists because a submitter proved control of the host at
// submit time, and this script never creates one. So a submission without a
// verified claim has no card, and this exits 0 having done nothing — the same
// as when the Blobs credentials are not configured. Neither case is a failure
// of the intake it follows.
//
// The card shows tool names and descriptions authored by the scanned site.
// They are copied verbatim into the record and escaped where they are
// rendered; nothing here interprets them.
import fs from 'node:fs'
import path from 'node:path'
import { pathToFileURL } from 'node:url'
import { getStore } from '@netlify/blobs'
import { canonicalHost } from './lib/directory'
import { expiresAfter, TRY_STORE, type TryRecord, type TryScan } from '../netlify/lib/try-record'
import type { ScanReport } from '../src/types/directory'

export const USAGE = 'usage: atlas-card --host <host> --scan <scan report json>'

/** Conditional writes lose a race at most this often before we give up. */
export const CARD_WRITE_ATTEMPTS = 3

export interface CardArgs {
  host: string
  scan: string
}

export function parseArgs(argv: string[]): CardArgs {
  const out: CardArgs = { host: '', scan: '' }
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i]
    const next = () => argv[++i] ?? ''
    if (a === '--host') out.host = next()
    else if (a === '--scan') out.scan = next()
    else throw new Error(`unknown argument ${a}\n${USAGE}`)
  }
  if (!out.scan) throw new Error(USAGE)
  return out
}

/**
 * The card's view of a scan: what was found, when, and on how many pages.
 * Deliberately less than the report — no checks, no counts breakdown, no
 * input schemas — because the card is a launch card, not a listing page.
 */
export function scanFromReport(report: ScanReport): TryScan {
  return {
    scannedAt: report.scannedAt,
    status: report.status,
    pages: report.counts.pages,
    tools: report.tools.map((tool) => ({
      name: tool.name,
      description: tool.description.trim(),
      kind: tool.risk,
      page: tool.page,
    })),
  }
}

/**
 * A scan is what keeps a card alive: the 30 days run from the scan date, not
 * from the claim, so a rescanned site keeps its card and an abandoned one
 * expires on its own.
 */
export function withScan(record: TryRecord, scan: TryScan): TryRecord {
  return { ...record, scan, expiresAt: expiresAfter(scan.scannedAt) }
}

export function readReport(file: string): ScanReport {
  const resolved = path.resolve(file)
  const parsed = JSON.parse(fs.readFileSync(resolved, 'utf-8')) as ScanReport
  if (!parsed || typeof parsed !== 'object' || !Array.isArray(parsed.tools)) {
    throw new Error(`${resolved} is not a scan report`)
  }
  return parsed
}

// --- Blobs ------------------------------------------------------------------
// Thin on purpose: everything above is pure and tested, and this is the part
// that needs a network and a token.

type Store = ReturnType<typeof getStore>

/** Merge the scan into the record on file. False when the host has no card. */
async function mergeScan(store: Store, host: string, scan: TryScan): Promise<boolean> {
  for (let attempt = 0; attempt < CARD_WRITE_ATTEMPTS; attempt += 1) {
    const entry = await store.getWithMetadata(host, { type: 'json' })
    if (entry === null) return false
    const record = withScan(entry.data as TryRecord, scan)
    // Same read-then-conditional-write discipline the submit function uses:
    // a resubmission for this host may have replaced the record since the read.
    const written = await store.setJSON(host, record, entry.etag ? { onlyIfMatch: entry.etag } : undefined)
    if (written.modified) return true
  }
  return false
}

export async function run(argv: string[]): Promise<number> {
  const args = parseArgs(argv)
  const report = readReport(args.scan)
  const host = canonicalHost(args.host || report.host)
  if (!host) throw new Error(`no host: pass --host, or scan a report that carries one\n${USAGE}`)

  const siteID = process.env.NETLIFY_SITE_ID?.trim()
  const token = process.env.NETLIFY_AUTH_TOKEN?.trim()
  if (!siteID || !token) {
    console.log('  no NETLIFY_SITE_ID / NETLIFY_AUTH_TOKEN: skipped the launch card')
    return 0
  }

  const updated = await mergeScan(getStore({ name: TRY_STORE, siteID, token }), host, scanFromReport(report))
  console.log(updated ? `  updated the launch card for ${host}` : `  no launch card for ${host}: nothing to update`)
  return 0
}

const entry = process.argv[1]
if (entry && import.meta.url === pathToFileURL(entry).href) {
  run(process.argv.slice(2))
    .then((code) => {
      process.exitCode = code
    })
    .catch((err) => {
      console.error('atlas-card failed:\n', err instanceof Error ? err.message : err)
      process.exit(1)
    })
}
