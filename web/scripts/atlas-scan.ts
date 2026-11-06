// Scan one site for WebMCP tools and write the report.
//
//   pnpm atlas:scan https://example.com/ [--out report.json] [--max-pages 3]
//                   [--intent "Find pricing"] [--path /docs --path /pricing]
//                   [--allow-local] [--md report.md]
//
// Drives a `sightmap browser` session (the CLI must be on PATH or in
// $SIGHTMAP_BIN). Discovery only: the scan enumerates registered tools and
// never calls one.
// See scripts/lib/scan.ts for what it does and src/data/directory/README.md
// for the report contract.
import fs from 'node:fs'
import path from 'node:path'
import { pathToFileURL } from 'node:url'
import { scanSite } from './lib/scan'
import { scanReportMarkdown } from './lib/directory-markdown'

export function parseArgs(argv: string[]) {
  const out = { url: '', out: '', md: '', maxPages: 3, intent: '', paths: [] as string[], allowLocal: false }
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i]
    const next = () => argv[++i] ?? ''
    if (a === '--out' || a === '-o') out.out = next()
    else if (a === '--md') out.md = next()
    else if (a === '--max-pages') out.maxPages = Number(next())
    else if (a === '--intent') out.intent = next()
    else if (a === '--path') out.paths.push(next())
    else if (a === '--allow-local') out.allowLocal = true
    else if (a.startsWith('-')) throw new Error(`unknown flag ${a}`)
    else if (!out.url) out.url = a
    else throw new Error(`unexpected argument ${a}`)
  }
  if (!out.url) throw new Error('usage: atlas-scan <url> [--out file] [--max-pages n] [--intent text] [--path /p]')
  return out
}

async function main() {
  const args = parseArgs(process.argv.slice(2))
  console.log(`  scanning ${args.url}`)
  const report = await scanSite({
    url: args.url,
    maxPages: args.maxPages,
    intent: args.intent,
    paths: args.paths,
    allowLocal: args.allowLocal,
    log: (line) => console.log(line),
  })
  const json = JSON.stringify(report, null, 2)
  if (args.out) {
    fs.mkdirSync(path.dirname(path.resolve(args.out)), { recursive: true })
    fs.writeFileSync(args.out, `${json}\n`)
    console.log(`  wrote ${args.out}`)
  } else {
    process.stdout.write(`${json}\n`)
  }
  if (args.md) {
    fs.mkdirSync(path.dirname(path.resolve(args.md)), { recursive: true })
    fs.writeFileSync(args.md, scanReportMarkdown(report))
    console.log(`  wrote ${args.md}`)
  }
  console.log(
    `\n  ${report.status}: ${report.counts.tools} tool(s) on ${report.counts.pages} page(s), surface ${report.surface}` +
      ` (${report.counts.read} read / ${report.counts.action} action / ${report.counts.sensitive} sensitive)`
  )
}

const entry = process.argv[1]
if (entry && import.meta.url === pathToFileURL(entry).href) {
  main().catch((err) => {
    console.error('Scan failed:\n', err instanceof Error ? err.message : err)
    process.exit(1)
  })
}
