// The `/atlas/<slug>/badge.svg` a listed site can embed in its own README.
//
// A pure string builder on purpose: no canvas, no font metrics, no network.
// The badge is generated at build time next to the rest of public/atlas/, so
// it has to be reproducible from the listing alone — and a takedown has to be
// able to delete it by wiping the directory, which is only true if nothing
// caches it elsewhere.
//
// Everything that reaches the SVG is escaped.

/** Colour of the right-hand block, by how the badge should read. */
export type BadgeTone = 'tools' | 'none'

const TONE: Record<BadgeTone, string> = {
  // Green: the scan found callable tools. Grey: it did not, which is an
  // observation and not a failure, so it must not render as red.
  tools: '#0a7d43',
  none: '#6b7280',
}

const LABEL_BG = '#41474e'

// Approximate advance widths for 11px Verdana/DejaVu Sans, the font stack
// every badge renderer uses. Exact metrics would need the font itself; being
// a pixel or two wide only adds padding, while being narrow would clip text,
// so every estimate here rounds up rather than down.
const NARROW = new Set(" '\"!|.,:;iIjlft[]()".split(''))
const WIDE = new Set('mMwW@%—·'.split(''))

export function textWidth(text: string): number {
  let width = 0
  for (const ch of text) {
    if (NARROW.has(ch)) width += 3.6
    else if (WIDE.has(ch)) width += 9.5
    else if (ch >= 'A' && ch <= 'Z') width += 7.8
    else width += 6.6
  }
  return Math.ceil(width)
}

/** Escapes text for both SVG text nodes and attribute values. */
export function escapeSvg(text: string): string {
  return text
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&apos;')
}

export interface BadgeOptions {
  label: string
  value: string
  tone?: BadgeTone
}

/**
 * A flat, two-block badge: dark label on the left, coloured value on the
 * right. The shape (20px tall, 3px radius, 11px text, the shields.io gradient
 * overlay) is the convention every README badge already follows, so this one
 * lines up with the others in a row instead of standing a pixel taller.
 */
export function badgeSvg({ label, value, tone = 'tools' }: BadgeOptions): string {
  // Collapse whitespace first: a newline inside an SVG text node renders as a
  // space but breaks the width estimate the layout is built from.
  const left = label.replace(/\s+/g, ' ').trim()
  const right = value.replace(/\s+/g, ' ').trim()

  const pad = 10
  const leftWidth = textWidth(left) + pad * 2
  const rightWidth = textWidth(right) + pad * 2
  const total = leftWidth + rightWidth

  // Centred in each block; the shadow text sits one pixel below (y+1).
  const leftX = leftWidth / 2
  const rightX = leftWidth + rightWidth / 2

  const alt = `${left}: ${right}`
  const l = escapeSvg(left)
  const r = escapeSvg(right)
  const a = escapeSvg(alt)

  return `<svg xmlns="http://www.w3.org/2000/svg" width="${total}" height="20" viewBox="0 0 ${total} 20" role="img" aria-label="${a}">
  <title>${a}</title>
  <linearGradient id="s" x2="0" y2="100%">
    <stop offset="0" stop-color="#fff" stop-opacity=".1"/>
    <stop offset="1" stop-opacity=".1"/>
  </linearGradient>
  <clipPath id="r">
    <rect width="${total}" height="20" rx="3" fill="#fff"/>
  </clipPath>
  <g clip-path="url(#r)">
    <rect width="${leftWidth}" height="20" fill="${LABEL_BG}"/>
    <rect x="${leftWidth}" width="${rightWidth}" height="20" fill="${TONE[tone]}"/>
    <rect width="${total}" height="20" fill="url(#s)"/>
  </g>
  <g fill="#fff" text-anchor="middle" font-family="Verdana,DejaVu Sans,Geneva,sans-serif" font-size="11">
    <text x="${leftX}" y="15" fill="#010101" fill-opacity=".3">${l}</text>
    <text x="${leftX}" y="14">${l}</text>
    <text x="${rightX}" y="15" fill="#010101" fill-opacity=".3">${r}</text>
    <text x="${rightX}" y="14">${r}</text>
  </g>
</svg>
`
}

/** What a listing's badge needs: how many tools, and when they were seen. */
export interface BadgeListing {
  counts: { tools: number }
  scannedAt: string
}

/**
 * The badge for one directory listing.
 *
 * The badge sits in someone else's README, away from the listing that would
 * qualify it, so it says only who looked and what was seen: the label is the
 * name of the party making the observation, and the value is the observation
 * and its date. "Detected" rather than a bare count, so the badge cannot be
 * read as Sightmap vouching for the site.
 */
export function listingBadgeSvg(listing: BadgeListing): string {
  const tools = listing.counts.tools
  const date = listing.scannedAt.slice(0, 10)
  const seen = tools === 0 ? 'no tools' : `${tools} tool${tools === 1 ? '' : 's'}`
  return badgeSvg({ label: 'Sightmap', value: `${seen} detected · ${date}`, tone: tools > 0 ? 'tools' : 'none' })
}
