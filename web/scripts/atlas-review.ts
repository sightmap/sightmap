// Review a scan artifact.
//
//   pnpm atlas:review <scan.json> [--out review.json] [--heuristic]
//
// With ANTHROPIC_API_KEY set (and without --heuristic) the scan is read by
// Claude through the SDK's structured outputs; otherwise, and on any API
// error, the heuristic review stands and says so in its notes. Either way the
// output is the same `Review` shape, which scripts/atlas-listing.ts consumes.
// See scripts/lib/review.ts for the merge rules and src/data/directory/README.md
// for the contract.
import fs from 'node:fs'
import path from 'node:path'
import { pathToFileURL } from 'node:url'
import { ScanReportSchema } from './lib/directory'
import { claudeReview, heuristicReview, type Review } from './lib/review'
import type { ScanReport } from '../src/types/directory'

export function parseArgs(argv: string[]) {
  const out = { scan: '', out: '', heuristic: false }
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i]
    const next = () => argv[++i] ?? ''
    if (a === '--out' || a === '-o') out.out = next()
    else if (a === '--heuristic') out.heuristic = true
    else if (a.startsWith('-')) throw new Error(`unknown flag ${a}`)
    else if (!out.scan) out.scan = a
    else throw new Error(`unexpected argument ${a}`)
  }
  if (!out.scan) throw new Error('usage: atlas-review <scan.json> [--out review.json] [--heuristic]')
  return out
}

/** Reads and validates a scan report written by scripts/atlas-scan.ts. */
export function readScanFile(file: string): ScanReport {
  const parsed = ScanReportSchema.safeParse(JSON.parse(fs.readFileSync(file, 'utf-8')))
  if (!parsed.success) {
    const issues = parsed.error.issues.map((i) => `    - ${i.path.join('.') || '(root)'}: ${i.message}`).join('\n')
    throw new Error(`invalid scan report ${file}:\n${issues}`)
  }
  return parsed.data as ScanReport
}

export function reviewSummary(review: Review): string {
  const kinds = { read: 0, action: 0, sensitive: 0 }
  for (const t of review.tools) kinds[t.kind]++
  return [
    `  ${review.recommendation} (${review.source} review)`,
    `  ${review.description}`,
    `  category ${review.category} · type ${review.type} · ${review.tools.length} tool(s): ${kinds.read} read / ${kinds.action} action / ${kinds.sensitive} sensitive`,
    review.suspicious.length > 0
      ? `  ${review.suspicious.length} suspicious warning(s): ${review.suspicious.join('; ')}`
      : '  no suspicious tool metadata flagged',
    `  ${review.suggested_journeys.length} suggested read-only journey(s)`,
    review.notes ? `  ${review.notes}` : '',
  ]
    .filter(Boolean)
    .join('\n')
}

async function main() {
  const args = parseArgs(process.argv.slice(2))
  const report = readScanFile(args.scan)
  const review = args.heuristic ? heuristicReview(report) : await claudeReview(report, { log: (l) => console.log(l) })

  if (args.out) {
    fs.mkdirSync(path.dirname(path.resolve(args.out)), { recursive: true })
    fs.writeFileSync(args.out, `${JSON.stringify(review, null, 2)}\n`)
    console.log(`  wrote ${args.out}`)
  } else {
    process.stdout.write(`${JSON.stringify(review, null, 2)}\n`)
  }
  console.log(`\n${reviewSummary(review)}`)
}

const entry = process.argv[1]
if (entry && import.meta.url === pathToFileURL(entry).href) {
  main().catch((err) => {
    console.error('Review failed:\n', err instanceof Error ? err.message : err)
    process.exit(1)
  })
}
