// Markdown renderings of scan reports and listings: the report a submitter
// receives, and the `/atlas/<slug>.md` machine twin of a directory listing.
// Everything a page registered (names, descriptions) is untrusted text and
// is rendered inside code spans or escaped, never as markdown.
import type { DirectoryListing, ScanReport, ToolKind } from '../../src/types/directory'
import { SITE_URL } from './site'

export const KIND_LABEL: Record<ToolKind, string> = {
  read: 'Read',
  action: 'Action',
  sensitive: 'Sensitive',
}

// Backticks would end a code span, and a newline would start a new block.
const code = (s: string): string => `\`${s.replace(/`/g, "'").replace(/\s+/g, ' ')}\``
const plain = (s: string): string => s.replace(/[\\`*_{}[\]()#+\-!<>|]/g, (m) => `\\${m}`).replace(/\s+/g, ' ')

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

Scanned ${r.scannedAt.slice(0, 10)} by ${r.scanner.name} ${r.scanner.version}. Discovery only: tools were enumerated, none were executed.

- WebMCP detected: ${r.status === 'tools-found' || r.status === 'needs-review' ? 'yes' : 'no'}
- Surface: ${r.surface}
- Status: ${r.status}
- Pages checked: ${r.counts.pages}
- Tools discovered: ${r.counts.tools} (${r.counts.read} read, ${r.counts.action} action, ${r.counts.sensitive} sensitive; ${r.counts.declarative} declarative)
${r.intent ? `- Submitted intent: ${plain(r.intent)}\n` : ''}
## Checks

${checks}

## Tools

Classification is Atlas's own heuristic, corrected by a maintainer on review. It is not a claim the site makes.

${tools}

## Pages

${pages}
${notes}
This scan is evidence, not a safety certification. See ${SITE_URL}/atlas for how listings are reviewed.
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
- Tools: ${l.counts.tools} (${l.counts.read} read, ${l.counts.action} action, ${l.counts.sensitive} sensitive)
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

Classification is Atlas's own, corrected on review. A listing is evidence a scan found these tools on the date shown; it is not a safety certification.
`
}
