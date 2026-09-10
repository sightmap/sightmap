// The unlisted launch card served at /try/<host>, as pure string builders.
//
// The card exists so a site that has proved domain control can be shared the
// day it ships, without waiting for review. That makes the wording load-
// bearing: the page reports two observations — that someone controlling the
// host asked for a scan, and what the scanner saw on a date — and nothing
// else. It never calls the site a demo worth trusting, and it never fetches,
// proxies, frames or screenshots it: everything below is built from the stored
// record alone.
//
// Everything that reaches the HTML is escaped as text, because a tool name and
// description come from a page we do not control and are the one part of the
// record an attacker chooses freely.
import { REPORT_EMAIL, SITE_URL } from '../../scripts/lib/site.ts'
import { KIND_LABEL } from '../../src/types/directory.ts'
import { isExpired, type TryRecord, type TryTool } from './try-record.ts'

/** Placeholder social image; see the note in the repo for replacing it. */
export const TRY_OG_IMAGE = '/try-og.png'

export interface TryCardOptions {
  /** Absolute origin the card is served from, for the share and social URLs. */
  siteUrl?: string
  reportEmail?: string
}

/**
 * Escapes a string for both HTML text and double-quoted attribute values.
 * Goes one further than `esc` in scripts/lib/site.ts by escaping the
 * apostrophe too, so a description is safe to drop into an attribute however
 * that attribute ends up quoted.
 */
export function escapeHtml(text: string): string {
  return text
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;')
}

/** The date a status line shows: the calendar day of an ISO timestamp. */
function day(iso: string): string {
  return iso.slice(0, 10)
}

/**
 * The status line, verbatim. Two sentences at most, both of them observations
 * with a date attached. Nothing here may grow into "verified site", "safe" or
 * "reviewed" — the second line says the opposite in as many words.
 */
export function statusLine(record: TryRecord): string {
  if (!record.scan) return `Domain control verified ${day(record.claimedAt)}. Scan pending.`
  return `${record.scan.tools.length} tools detected on ${day(record.scan.scannedAt)}. Domain control verified. Not reviewed by Sightmap.`
}

/**
 * The prompt a reader pastes into an agent. Built from the first read-kind
 * tool: an action or a sensitive tool would have the reader's agent change
 * something on a stranger's site on the strength of a card that vouches for
 * nothing. Null when the scan found no read tool.
 */
export function promptFor(record: TryRecord): string | null {
  const tool = record.scan?.tools.find((t) => t.kind === 'read')
  if (!tool) return null
  return `Open https://${record.host} and call ${tool.name} — ${tool.description}`
}

/**
 * The share text. It names WebMCP agents as the audience and never a specific
 * client or assistant: which agent a reader points at the site is their
 * choice, and naming one would read as an endorsement in both directions.
 */
export function shareText(record: TryRecord, cardUrl: string): string {
  const count = record.scan?.tools.length
  if (count === undefined) return `${record.host} is claimed on Sightmap for WebMCP agents. ${cardUrl}`
  return `${count} WebMCP tools detected on ${record.host}, callable by WebMCP agents. ${cardUrl}`
}

const STYLE = `
:root { color-scheme: light dark }
* { box-sizing: border-box }
body {
  margin: 0;
  padding: 24px 16px;
  background: #f5f5f4;
  color: #1c1917;
  font: 15px/1.5 ui-sans-serif, system-ui, -apple-system, Segoe UI, Roboto, sans-serif;
}
main {
  max-width: 34rem;
  margin: 0 auto;
  padding: 20px;
  border: 1px solid #1c1917;
  background: #fff;
}
h1 {
  margin: 0;
  font-size: 1.15rem;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  overflow-wrap: anywhere;
}
.tag {
  display: inline-block;
  margin: 12px 0 0;
  padding: 2px 8px;
  border: 1px solid #1c1917;
  font-size: 0.75rem;
  letter-spacing: 0.06em;
  text-transform: uppercase;
}
.status { margin: 12px 0 0; font-weight: 600 }
h2 { margin: 24px 0 8px; font-size: 0.8rem; letter-spacing: 0.06em; text-transform: uppercase; color: #57534e }
ul { margin: 0; padding: 0; list-style: none }
li { padding: 8px 0; border-top: 1px solid #e7e5e4 }
.tool-name { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; overflow-wrap: anywhere }
.kind { margin-left: 6px; font-size: 0.75rem; color: #57534e }
.desc { margin: 2px 0 0; color: #44403c; overflow-wrap: anywhere }
.page { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 0.75rem; color: #78716c }
pre {
  margin: 0;
  padding: 12px;
  border: 1px dashed #78716c;
  background: #fafaf9;
  font-size: 0.85rem;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  -webkit-user-select: all;
  user-select: all;
}
.links { margin: 20px 0 0; padding: 0; list-style: none }
.links li { border-top: 0; padding: 4px 0 }
.hint { margin: 6px 0 0; font-size: 0.75rem; color: #78716c }
.note { margin: 20px 0 0; padding-top: 12px; border-top: 1px solid #e7e5e4; font-size: 0.85rem; color: #57534e }
a { color: #1c1917 }
@media (prefers-color-scheme: dark) {
  body { background: #0c0a09; color: #f5f5f4 }
  main { background: #1c1917; border-color: #78716c }
  .tag { border-color: #a8a29e }
  h2, .kind, .page, .hint, .note { color: #a8a29e }
  .desc { color: #d6d3d1 }
  li { border-top-color: #44403c }
  pre { background: #0c0a09; border-color: #57534e }
  a { color: #f5f5f4 }
}
`.trim()

/** Every link off sightmap.org carries the same rel, including the mailto. */
const REL = 'nofollow noopener'

function shell(title: string, head: string, body: string): string {
  return `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="robots" content="noindex,nofollow">
<title>${title}</title>
${head}<style>${STYLE}</style>
</head>
<body>
<main>
${body}
</main>
</body>
</html>
`
}

function toolItem(tool: TryTool): string {
  const label = KIND_LABEL[tool.kind] ?? tool.kind
  return `<li><span class="tool-name">${escapeHtml(tool.name)}</span><span class="kind">${escapeHtml(label)}</span>
<p class="desc">${escapeHtml(tool.description)}</p>
<span class="page">${escapeHtml(tool.page)}</span></li>`
}

/**
 * The card itself.
 *
 * Deliberately unlike an Atlas listing page: no navigation, no screenshot, no
 * badge, one bordered box. A reader who has seen both should never mistake
 * this page for a listing, because a listing means a maintainer looked and
 * this page means nobody has.
 */
export function renderTryCard(record: TryRecord, opts: TryCardOptions = {}): string {
  const siteUrl = (opts.siteUrl ?? SITE_URL).replace(/\/+$/, '')
  const reportEmail = opts.reportEmail ?? REPORT_EMAIL
  const host = record.host
  const origin = `https://${host}`
  const escOrigin = escapeHtml(origin)
  const cardUrl = `${siteUrl}/try/${encodeURIComponent(host)}`
  const title = `${escapeHtml(host)} · Sightmap try`

  const share = `https://twitter.com/intent/tweet?text=${encodeURIComponent(shareText(record, cardUrl))}`
  const report = `mailto:${reportEmail}?subject=Report%20${encodeURIComponent(host)}`
  const prompt = promptFor(record)
  const tools = record.scan?.tools ?? []

  const head = `<meta name="description" content="An unlisted card for ${escapeHtml(host)}. Not reviewed by Sightmap.">
<meta property="og:type" content="website">
<meta property="og:title" content="${title}">
<meta property="og:description" content="An unlisted card. Not reviewed by Sightmap.">
<meta property="og:url" content="${escapeHtml(cardUrl)}">
<meta property="og:image" content="${escapeHtml(siteUrl + TRY_OG_IMAGE)}">
<meta name="twitter:card" content="summary_large_image">
`

  // The origin leads the page, and is repeated beside every link that leaves
  // sightmap.org, so a reader is never one click from a stranger's site
  // without having read which site it is.
  const parts: string[] = [
    `<h1>${escOrigin}</h1>`,
    `<p class="tag">Unreviewed</p>`,
    `<p class="status">${escapeHtml(statusLine(record))}</p>`,
  ]

  if (tools.length > 0) {
    parts.push(`<h2>Tools the scanner saw</h2>`, `<ul>${tools.map(toolItem).join('\n')}</ul>`)
  }

  if (prompt) {
    parts.push(
      `<h2>Try it</h2>`,
      `<pre>${escapeHtml(prompt)}</pre>`,
      `<p class="hint">Click the prompt to select all of it.</p>`
    )
  }

  parts.push(`<ul class="links">
<li><a href="${escOrigin}" rel="${REL}">Open ${escOrigin}</a></li>
<li><a href="${escapeHtml(share)}" rel="${REL}">Share this card for ${escOrigin}</a></li>
<li><a href="${escapeHtml(report)}" rel="${REL}">Report ${escOrigin}</a></li>
</ul>`)

  parts.push(
    `<p class="note">A site appears in the Sightmap Atlas only after a maintainer reviews it. <a href="/atlas">Browse the Atlas</a>.</p>`
  )

  return shell(title, head, parts.join('\n'))
}

/** 410: the card was withdrawn. Says that much and no more. */
export function renderGone(): string {
  return shell(
    'Card removed · Sightmap',
    '',
    `<h1>Card removed</h1>
<p class="status">This card was removed.</p>
<p class="note">Nothing here says anything about the site it pointed at. <a href="/atlas">Browse the Atlas</a>.</p>`
  )
}

/** 404: no card for this host — never a hint about whether one ever existed. */
export function renderNotFound(): string {
  return shell(
    'No card · Sightmap',
    '',
    `<h1>No card</h1>
<p class="status">There is no card at this address.</p>
<p class="note">A site appears in the Sightmap Atlas only after a maintainer reviews it. <a href="/atlas">Browse the Atlas</a>.</p>`
  )
}

export type TryDecision =
  | { kind: 'not-found' }
  | { kind: 'listed'; slug: string }
  | { kind: 'gone' }
  | { kind: 'card'; record: TryRecord }

export interface TryDecisionInputs {
  /** Whether the requested host passes `preflightUrl`'s shape rules. */
  preflightOk: boolean
  /** The slug from /atlas/hosts/<host>.json, when the host is already listed. */
  listedSlug: string | null
  quarantined: boolean
  record: TryRecord | null
  now?: number
}

/**
 * The whole decision for GET /try/<host>, kept out of the function so the
 * order of the checks is testable without a runtime.
 *
 * The caller may skip the lookups an earlier check makes unnecessary: an input
 * it never gathered is passed as `null` / `false`, which can only ever be read
 * after a check that has already decided. A storage failure arrives the same
 * way, so an unreadable store reads as "no card" rather than as a 500.
 */
export function decideTryCard(inputs: TryDecisionInputs): TryDecision {
  if (!inputs.preflightOk) return { kind: 'not-found' }
  if (inputs.listedSlug) return { kind: 'listed', slug: inputs.listedSlug }
  if (inputs.quarantined) return { kind: 'gone' }
  if (!inputs.record) return { kind: 'not-found' }
  if (isExpired(inputs.record, inputs.now ?? Date.now())) return { kind: 'not-found' }
  return { kind: 'card', record: inputs.record }
}
