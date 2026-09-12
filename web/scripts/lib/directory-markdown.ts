// Markdown renderings of scan reports and listings: the report a submitter
// receives, and the `/atlas/<slug>.md` machine twin of a directory listing.
// Everything a page registered (names, descriptions) is untrusted text and
// is rendered inside code spans or escaped, never as markdown.
import { KIND_LABEL, type DirectoryListing, type ScanReport } from '../../src/types/directory'
import { SITE_URL } from './site'

/** Untrusted text as a code span. A backtick would end it; a newline would end the block. */
export const code = (s: string): string => `\`${s.replace(/`/g, "'").replace(/\s+/g, ' ')}\``
/** Untrusted text as prose: every markdown metacharacter escaped. */
export const plain = (s: string): string => s.replace(/[\\`*_{}[\]()#+\-!<>|]/g, (m) => `\\${m}`).replace(/\s+/g, ' ')

export function scanReportMarkdown(r: ScanReport): string {
  const checks = r.checks.map((c) => `- ${c.ok ? '✓' : '!'} ${c.label} (${c.detail})`).join('\n')
  const tools =
    r.tools.length === 0
      ? '_No tools registered on the pages checked._'
      : r.tools
          .map(
            (t) =>
              `- ${code(t.name)} — ${KIND_LABEL[t.risk]} · ${t.impl} · on ${t.pages.map(code).join(', ')}` +
              (t.description ? `\n  ${plain(t.description)}` : '') +
              (t.warnings.length ? `\n  Warnings: ${t.warnings.map(plain).join('; ')}` : '')
          )
          .join('\n')
  const pages = r.pages
    .map((p) => `- ${code(p.path)} — ${p.surface}, ${p.tools.length} tool(s)${p.error ? ` — ${plain(p.error)}` : ''}`)
    .join('\n')
  const notes = r.notes.length ? `\n## Notes\n\n${r.notes.map((n) => `- ${plain(n)}`).join('\n')}\n` : ''
  return `# Atlas scan: ${plain(r.host)}

Scanned ${r.scannedAt.slice(0, 10)} by ${r.scanner.name} ${r.scanner.version}. Discovery only: tools were enumerated from the pages, none were called.

- WebMCP detected: ${r.status === 'tools-found' || r.status === 'needs-review' ? 'yes' : 'no'}
- Surface: ${r.surface}
- Status: ${r.status}
- Pages checked: ${r.counts.pages}
- Tools detected: ${r.counts.tools} (${r.counts.read} read, ${r.counts.action} action, ${r.counts.sensitive} sensitive; ${r.counts.declarative} declarative)
${r.intent ? `- Submitted intent: ${plain(r.intent)}\n` : ''}
## Checks

${checks}

## Tools

Classification is Atlas's own heuristic, corrected by a maintainer on review. It is not a claim the site makes.

${tools}

## Pages

${pages}
${notes}
This scan records what the pages above registered on ${r.scannedAt.slice(0, 10)}. No tool was called, and nothing here is a review of the site. A listing appears in the Atlas only after a maintainer reviews it: ${SITE_URL}/atlas.
`
}

export function listingMarkdown(l: DirectoryListing): string {
  const tools = l.tools
    .map((t) => `- ${code(t.name)} — ${KIND_LABEL[t.kind]}, on ${code(t.page)}${t.description ? `: ${plain(t.description)}` : ''}`)
    .join('\n')
  const checks = l.report.checks.map((c) => `- ${c.ok ? '✓' : '!'} ${c.label} (${c.detail})`).join('\n')
  const journeys = l.suggested_journeys.map((j) => `- ${plain(j.intent)}${j.tools.length ? ` — ${j.tools.map(code).join(' → ')}` : ''}`).join('\n')
  const journey = l.journey
    ? `\n## Stored journey\n\n- Intent: ${plain(l.journey.intent)}\n- Outcome: ${l.journey.outcome} on ${l.journey.ran_at}\n- Tools called: ${l.journey.tools_called.map(code).join(', ') || 'none'}\n- Duration: ${l.journey.duration_ms} ms\n${l.journey.notes ? `- Notes: ${plain(l.journey.notes)}\n` : ''}`
    : ''
  const drift = l.drift
    ? `\n## Since ${l.drift.since}\n\n- Added: ${l.drift.added.map(code).join(', ') || 'none'}\n- Removed: ${l.drift.removed.map(code).join(', ') || 'none'}\n`
    : ''
  return `# ${plain(l.name)}

${plain(l.description)}

- Site: ${l.url}
- Type: ${l.type}${l.built_with_sightkick ? ' · built with Sightkick' : ''}
- Category: ${l.category}
- WebMCP surface: ${l.surface} (${l.status})
- Tools detected: ${l.counts.tools} (${l.counts.read} read, ${l.counts.action} action, ${l.counts.sensitive} sensitive)
- Pages checked: ${l.counts.pages}
- Last scanned: ${l.scannedAt.slice(0, 10)}
- Listed: ${l.added}, updated ${l.updated}

## Tools

${tools || '_No tools listed._'}

## Checks

${checks}
${l.strengths.length ? `\n## Strong\n\n${l.strengths.map((s) => `- ${plain(s)}`).join('\n')}\n` : ''}${l.improvements.length ? `\n## Improve\n\n${l.improvements.map((s) => `- ${plain(s)}`).join('\n')}\n` : ''}${journeys ? `\n## Suggested journeys\n\n${journeys}\n` : ''}${journey}${drift}
## Machine-readable

- ${SITE_URL}/atlas/sites/${l.slug}.json
- ${SITE_URL}/atlas/sites/${l.slug}/tools.json
- ${SITE_URL}/atlas/scans/${l.slug}.json
- ${SITE_URL}/atlas/directory.json
- Building blueprint: ${SITE_URL}/atlas/sites/${l.slug}/blueprint.json

Classification is Atlas's own, corrected by a maintainer on review. A listing records the tools a scan detected on the date shown, and says nothing about the site beyond that.
`
}
