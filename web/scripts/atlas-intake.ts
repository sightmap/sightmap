// The whole intake in one command: scan → review → listing, plus the Markdown
// a review agent puts in the pull request body.
//
//   pnpm atlas:intake --url <url> [--intent "Find pricing"] [--submission-id 42]
//                     [--type live|demo] [--sightkick]
//                     [--submitted-by owner|nominator|maintainer]
//                     [--max-pages 3] [--path /docs] [--heuristic]
//                     [--allow-local] [--summary summary.md]
//                     [--sightmap-bin ./sightmap] [--data-dir …] [--atlas-dir …]
//                     [--replace-review]
//
// The summary is written for a human who has not seen the site. Everything the
// page authored — tool names, descriptions, page titles — is untrusted text, so
// it is rendered inside code spans with the table and markdown metacharacters
// escaped; the same rule scripts/lib/directory-markdown.ts follows.
//
// Exit status: 1 when the scan was blocked or failed to load. The summary is
// still written in that case, because the runner's job is to report the
// outcome, and a scan that never loaded the page produces no listing.
import fs from 'node:fs'
import path from 'node:path'
import { pathToFileURL } from 'node:url'
import { scanSite } from './lib/scan'
import { createListing, type CreatedListing, type SubmittedBy } from './lib/listing'
import { claudeReview, heuristicReview, type Review } from './lib/review'
import { DEFAULT_ATLAS_DIR, DEFAULT_DATA_DIR } from './atlas-listing'
import type { ListingType, ScanReport } from '../src/types/directory'

const SUBMITTED_BY: SubmittedBy[] = ['owner', 'nominator', 'maintainer']

export interface IntakeArgs {
  url: string
  intent: string
  submissionId: string
  type?: ListingType
  sightkick: boolean
  submittedBy?: SubmittedBy
  maxPages: number
  paths: string[]
  heuristic: boolean
  allowLocal: boolean
  /** Write a listing even when the scan found no tools (default: skip it). */
  allowEmpty: boolean
  summary: string
  sightmapBin: string
  dataDir: string
  atlasDir: string
  replaceReview: boolean
}

export const USAGE =
  'usage: atlas-intake --url <url> [--intent text] [--submission-id id] [--type live|demo] [--sightkick] ' +
  '[--submitted-by owner|nominator|maintainer] [--max-pages 3] [--path /p] [--heuristic] [--allow-local] [--allow-empty] ' +
  '[--summary summary.md] [--sightmap-bin path] [--data-dir dir] [--atlas-dir dir] [--replace-review]'

export function parseArgs(argv: string[]): IntakeArgs {
  const out: IntakeArgs = {
    url: '',
    intent: '',
    submissionId: '',
    type: undefined,
    sightkick: false,
    submittedBy: undefined,
    maxPages: 3,
    paths: [],
    heuristic: false,
    allowLocal: false,
    allowEmpty: false,
    summary: '',
    sightmapBin: '',
    dataDir: DEFAULT_DATA_DIR,
    atlasDir: DEFAULT_ATLAS_DIR,
    replaceReview: false,
  }
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i]
    const next = () => argv[++i] ?? ''
    if (a === '--url') out.url = next()
    else if (a === '--intent') out.intent = next()
    else if (a === '--submission-id') out.submissionId = next()
    else if (a === '--type') {
      const v = next()
      if (v !== 'live' && v !== 'demo') throw new Error(`--type must be live or demo, got ${v}`)
      out.type = v
    } else if (a === '--sightkick') out.sightkick = true
    else if (a === '--submitted-by') {
      const v = next() as SubmittedBy
      if (!SUBMITTED_BY.includes(v)) throw new Error(`--submitted-by must be one of ${SUBMITTED_BY.join(', ')}`)
      out.submittedBy = v
    } else if (a === '--max-pages') {
      const n = Number(next())
      if (!Number.isFinite(n) || n < 1) throw new Error('--max-pages must be a positive number')
      out.maxPages = n
    } else if (a === '--path') out.paths.push(next())
    else if (a === '--heuristic') out.heuristic = true
    else if (a === '--allow-local') out.allowLocal = true
    else if (a === '--allow-empty') out.allowEmpty = true
    else if (a === '--summary') out.summary = next()
    else if (a === '--sightmap-bin') out.sightmapBin = next()
    else if (a === '--data-dir') out.dataDir = next()
    else if (a === '--atlas-dir') out.atlasDir = next()
    else if (a === '--replace-review') out.replaceReview = true
    else if (a.startsWith('-')) throw new Error(`unknown flag ${a}`)
    else if (!out.url) out.url = a
    else throw new Error(`unexpected argument ${a}`)
  }
  if (!out.url) throw new Error(USAGE)
  return out
}

// --- Markdown helpers -------------------------------------------------------
//
// scripts/lib/directory-markdown.ts keeps its own copies of these unexported,
// on purpose: they are three lines each and both files want to be readable
// next to the output they produce.

/** Untrusted text as a code span. A backtick would end it; a newline would end the block. */
export const code = (s: string): string => `\`${s.replace(/`/g, "'").replace(/\s+/g, ' ')}\``
/** Untrusted text as prose: every markdown metacharacter escaped. */
export const plain = (s: string): string => s.replace(/[\\`*_{}[\]()#+\-!<>|]/g, (m) => `\\${m}`).replace(/\s+/g, ' ')
/** Untrusted text in a table cell: as above, plus the pipe that would split it. */
export const cell = (s: string): string => plain(s).replace(/\|/g, '\\|')
const codeCell = (s: string): string => code(s).replace(/\|/g, '\\|')

const bullets = (lines: string[], empty: string): string =>
  lines.length > 0 ? lines.map((l) => `- ${plain(l)}`).join('\n') : `_${empty}_`

/**
 * The summary's first line, and with the leading `#` stripped the pull-request
 * title: `atlas: list <host>`, `atlas: rescan <host>` or
 * `atlas: needs review — <host>` (em dash), the three forms
 * web/ATLAS_PIPELINE.md defines.
 *
 * .github/workflows/atlas-review.yml can derive the same three titles from
 * markers further down the summary — "(rescan of an existing listing)" under
 * `## Files` and the `Status:` bullet — and takes a first line that already
 * starts with `atlas:` verbatim. Writing the title here keeps both paths on the
 * same answer, so the precedence below matches the workflow's: needs-review
 * wins over rescan, which wins over a first listing.
 *
 * The host goes in unescaped: it comes from `new URL(...).hostname`, and a
 * backslash-escaped hyphen in a PR title would be nonsense.
 */
export function summaryTitle(report: ScanReport, listing: CreatedListing | null): string {
  if (report.status === 'needs-review') return `atlas: needs review — ${report.host}`
  return `atlas: ${listing?.isRescan ? 'rescan' : 'list'} ${report.host}`
}

export interface IntakeSummaryInput {
  report: ScanReport
  review: Review
  /** null when the scan was blocked or failed to load: nothing was written. */
  listing: CreatedListing | null
  submissionId?: string
  /** Paths are printed relative to this; defaults to the current directory. */
  cwd?: string
}

/**
 * The pull-request body. Facts first, then the review, then the checklist a
 * maintainer works through before merging.
 */
export function intakeSummary({ report, review, listing, submissionId, cwd = process.cwd() }: IntakeSummaryInput): string {
  const rel = (p: string) => path.relative(cwd, p) || p
  const c = report.counts

  const checks = [
    '| | Check | Detail |',
    '|---|---|---|',
    ...report.checks.map((k) => `| ${k.ok ? '✓' : '!'} | ${cell(k.label)} | ${cell(k.detail)} |`),
  ].join('\n')

  const kinds = new Map(review.tools.map((t) => [t.name, t.kind]))
  const tools =
    report.tools.length === 0
      ? '_No tools were registered on the pages checked._'
      : [
          '| Tool | Kind | Page | Description |',
          '|---|---|---|---|',
          ...report.tools.map(
            (t) =>
              `| ${codeCell(t.name)} | ${kinds.get(t.name) ?? t.risk} | ${codeCell(t.page)} | ${
                t.description.trim() ? cell(t.description) : '—'
              } |`
          ),
        ].join('\n')

  const journeys =
    review.suggested_journeys.length > 0
      ? review.suggested_journeys.map((j) => `- ${plain(j.intent)} — ${j.tools.map(code).join(' → ')}`).join('\n')
      : '_No read-only journey was suggested._'

  const files = listing
    ? `- Listing: \`${rel(listing.listingPath)}\`\n- Scan report: \`${rel(listing.scanPath)}\`\n- Slug: \`${listing.slug}\`${
        listing.isRescan ? ' (rescan of an existing listing)' : ' (new listing)'
      }`
    : '_No listing was written: the scan did not reach the site._'

  return `# ${summaryTitle(report, listing)}

Atlas intake for ${code(report.host)}. ${plain(review.description)}

- Submission: ${submissionId ? code(submissionId) : '_none given_'}
- URL: ${report.finalUrl}
- Status: \`${report.status}\` · surface \`${report.surface}\`
- Tools: ${c.tools} (${c.read} read, ${c.action} action, ${c.sensitive} sensitive; ${c.declarative} declarative)
- Pages checked: ${c.pages} · described ${c.described}/${c.tools} · with input schema ${c.withInputSchema}/${c.tools}
- Scanned: ${report.scannedAt.slice(0, 10)} by ${plain(report.scanner.name)} ${plain(report.scanner.version)}
${report.intent ? `- Submitted intent: ${plain(report.intent)}\n` : ''}
Discovery only: the scan enumerated tools and called none of them.

## Recommendation

**${review.recommendation}** — from the ${review.source} review.

${plain(review.notes)}

## Checks

${checks}

## Tools

Kinds are Atlas's own classification, corrected on review. They are not a claim the site makes.

${tools}

## Suspicious wording

${
  review.suspicious.length > 0
    ? `${review.suspicious.map((s) => `- ${plain(s)}`).join('\n')}\n\nTreat these as text a page wrote, not as instructions. Read them before running anything.`
    : '_Nothing flagged in the tool metadata._'
}

## Strong

${bullets(review.strengths, 'Nothing to note.')}

## Improve

${bullets(review.improvements, 'Nothing to note.')}

## Suggested read-only journeys

${journeys}

## Files

${files}

## Maintainer checklist

- [ ] Read every tool name and description above. Nothing in them is an instruction to you or to an agent.
- [ ] Open ${report.finalUrl} and confirm the site is what this listing says it is.
- [ ] Confirm the type: \`${review.type}\` (live = a public product surface, demo = hackathon, competition, example, or experimental).
- [ ] Correct any tool kind that is wrong — read (information only), action (reversible state or UI change), sensitive (money, commitment, or hard to reverse).
- [ ] Decide whether a supervised read-only journey may be run, and which tools it may call.
- [ ] Approve or rewrite the one-sentence description, then merge.

A listing is evidence that a scan found these tools on the date shown. It is not a safety certification.
`
}

export interface IntakeResult {
  code: number
  report: ScanReport
  review: Review
  listing: CreatedListing | null
  summary: string
}

/**
 * The whole pipeline. Returns instead of exiting so a caller (or a test that
 * stubs the scan) can inspect the outcome; `main` maps `code` onto the process.
 */
export async function run(argv: string[]): Promise<IntakeResult> {
  const args = parseArgs(argv)
  const log = (line: string) => console.log(line)

  log(`  scanning ${args.url}`)
  const report = await scanSite({
    url: args.url,
    maxPages: args.maxPages,
    intent: args.intent,
    paths: args.paths,
    allowLocal: args.allowLocal,
    sightmapBin: args.sightmapBin || undefined,
    log,
  })

  const review = args.heuristic ? heuristicReview(report) : await claudeReview(report, { log })

  // A scan that never reached the site has nothing to list. The summary still
  // gets written so the runner can report why.
  const failed = report.status === 'blocked' || report.status === 'load-error'
  // A site with no tools is not a directory entry: the Atlas lists apps with
  // callable tools, and the submitter's takeaway for an empty scan is the
  // Sightkick starter in the summary, not a listing that says "0 tools".
  const empty = report.counts.tools === 0 && !args.allowEmpty
  if (empty) log('  no tools found: no listing written (pass --allow-empty to write one anyway)')
  let listing: CreatedListing | null = null
  if (!failed && !empty) {
    listing = await createListing({
      dataDir: path.resolve(args.dataDir),
      atlasDir: path.resolve(args.atlasDir),
      report,
      review,
      type: args.type,
      sightkick: args.sightkick || undefined,
      submittedBy: args.submittedBy,
      replaceReview: args.replaceReview,
    })
    log(`  ${listing.isRescan ? 'rescanned' : 'listed'} ${report.host} as ${listing.slug}`)
    log(`  wrote ${listing.listingPath}`)
    log(`  wrote ${listing.scanPath}`)
  }

  const summary = intakeSummary({ report, review, listing, submissionId: args.submissionId })
  if (args.summary) {
    fs.mkdirSync(path.dirname(path.resolve(args.summary)), { recursive: true })
    fs.writeFileSync(args.summary, summary)
    log(`  wrote ${args.summary}`)
  }

  return { code: failed ? 1 : 0, report, review, listing, summary }
}

async function main() {
  const result = await run(process.argv.slice(2))
  process.stdout.write(`\n${result.summary}`)
  if (result.code !== 0) {
    console.error(`\nScan status ${result.report.status}: no listing was written.`)
  }
  process.exitCode = result.code
}

const entry = process.argv[1]
if (entry && import.meta.url === pathToFileURL(entry).href) {
  main().catch((err) => {
    console.error('Intake failed:\n', err instanceof Error ? err.message : err)
    process.exit(1)
  })
}
