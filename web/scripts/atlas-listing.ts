// Write a directory listing from a scan report and its review.
//
//   pnpm atlas:listing <scan.json> [--review review.json] [--type live|demo]
//                      [--sightkick] [--submitted-by owner|nominator|maintainer]
//                      [--data-dir src/data/directory] [--atlas-dir src/data/atlas]
//                      [--replace-review]
//
// Writes `<slug>.yaml` and files the report under `scans/<slug>/<date>.json`.
// Rerunning it for a host that is already listed is a rescan: same slug, same
// `added`, maintainer edits kept, `updated` bumped. Without --review the
// heuristic review runs, so this script never makes a network call.
// See scripts/lib/listing.ts and src/data/directory/README.md.
import fs from 'node:fs'
import path from 'node:path'
import { pathToFileURL } from 'node:url'
import { createListing, type SubmittedBy } from './lib/listing'
import { heuristicReview, type Review } from './lib/review'
import { readScanFile } from './atlas-review'
import type { ListingType } from '../src/types/directory'

export const DEFAULT_DATA_DIR = 'src/data/directory'
export const DEFAULT_ATLAS_DIR = 'src/data/atlas'

const SUBMITTED_BY: SubmittedBy[] = ['owner', 'nominator', 'maintainer']

export function parseArgs(argv: string[]) {
  const out = {
    scan: '',
    review: '',
    type: undefined as ListingType | undefined,
    sightkick: false,
    submittedBy: undefined as SubmittedBy | undefined,
    dataDir: DEFAULT_DATA_DIR,
    atlasDir: DEFAULT_ATLAS_DIR,
    replaceReview: false,
  }
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i]
    const next = () => argv[++i] ?? ''
    if (a === '--review') out.review = next()
    else if (a === '--type') {
      const v = next()
      if (v !== 'live' && v !== 'demo') throw new Error(`--type must be live or demo, got ${v}`)
      out.type = v
    } else if (a === '--sightkick') out.sightkick = true
    else if (a === '--submitted-by') {
      const v = next() as SubmittedBy
      if (!SUBMITTED_BY.includes(v)) throw new Error(`--submitted-by must be one of ${SUBMITTED_BY.join(', ')}`)
      out.submittedBy = v
    } else if (a === '--data-dir') out.dataDir = next()
    else if (a === '--atlas-dir') out.atlasDir = next()
    else if (a === '--replace-review') out.replaceReview = true
    else if (a.startsWith('-')) throw new Error(`unknown flag ${a}`)
    else if (!out.scan) out.scan = a
    else throw new Error(`unexpected argument ${a}`)
  }
  if (!out.scan) {
    throw new Error(
      'usage: atlas-listing <scan.json> [--review review.json] [--type live|demo] [--sightkick] ' +
        '[--submitted-by owner|nominator|maintainer] [--data-dir dir] [--replace-review]'
    )
  }
  return out
}

/** A review from disk, or the heuristic one when none was supplied. */
export function loadReview(file: string, report: Parameters<typeof heuristicReview>[0]): Review {
  if (!file) return heuristicReview(report)
  return JSON.parse(fs.readFileSync(file, 'utf-8')) as Review
}

async function main() {
  const args = parseArgs(process.argv.slice(2))
  const report = readScanFile(args.scan)
  const review = loadReview(args.review, report)

  const result = await createListing({
    dataDir: path.resolve(args.dataDir),
    atlasDir: path.resolve(args.atlasDir),
    report,
    review,
    type: args.type,
    sightkick: args.sightkick || undefined,
    submittedBy: args.submittedBy,
    replaceReview: args.replaceReview,
  })

  console.log(`  ${result.isRescan ? 'rescanned' : 'listed'} ${report.host} as ${result.slug}`)
  console.log(`  wrote ${result.listingPath}`)
  console.log(`  wrote ${result.scanPath}`)
}

const entry = process.argv[1]
if (entry && import.meta.url === pathToFileURL(entry).href) {
  main().catch((err) => {
    console.error('Listing failed:\n', err instanceof Error ? err.message : err)
    process.exit(1)
  })
}
