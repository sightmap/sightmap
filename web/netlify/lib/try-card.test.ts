// The card is rendered from a record whose tool names and descriptions were
// written by whoever controls the scanned page, and it is the one Sightmap
// page that says "not reviewed" out loud. So the tests below are mostly about
// two things: that nothing from the record can become markup, and that the
// status line still says exactly what it is allowed to say.

import { describe, expect, it } from 'vitest'
import {
  decideTryCard,
  escapeHtml,
  promptFor,
  renderGone,
  renderNotFound,
  renderTryCard,
  shareText,
  statusLine,
} from './try-card.ts'
import { expiresAfter, type TryRecord } from './try-record.ts'

const CLAIMED_AT = '2026-09-01T12:00:00.000Z'
const SCANNED_AT = '2026-09-08T09:30:00.000Z'

function record(overrides: Partial<TryRecord> = {}): TryRecord {
  return {
    v: 1,
    host: 'example.dev',
    url: 'https://example.dev/',
    submissionId: 'sub_123',
    claimedAt: CLAIMED_AT,
    expiresAt: expiresAfter(CLAIMED_AT),
    ...overrides,
  }
}

function scanned(): TryRecord {
  return record({
    scan: {
      scannedAt: SCANNED_AT,
      status: 'tools-found',
      pages: 4,
      tools: [
        { name: 'search_docs', description: 'Search the documentation.', kind: 'read', page: '/docs' },
        { name: 'add_to_cart', description: 'Add an item to the cart.', kind: 'action', page: '/shop' },
      ],
    },
  })
}

/** Every `<a href>` in a document, in source order. */
function anchors(html: string): { href: string; rel: string | null }[] {
  return [...html.matchAll(/<a\s+href="([^"]*)"([^>]*)>/g)].map((m) => ({
    href: m[1]!,
    rel: /rel="([^"]*)"/.exec(m[2]!)?.[1] ?? null,
  }))
}

describe('escapeHtml', () => {
  it('escapes every character that could close a tag or an attribute', () => {
    expect(escapeHtml(`<img src="x" onerror='y'>&`)).toBe(
      '&lt;img src=&quot;x&quot; onerror=&#39;y&#39;&gt;&amp;'
    )
  })

  it('escapes ampersands first so entities are not double-escaped', () => {
    expect(escapeHtml('&lt;')).toBe('&amp;lt;')
  })
})

describe('statusLine', () => {
  it('reports the claim and nothing else before a scan', () => {
    expect(statusLine(record())).toBe('Domain control verified 2026-09-01. Scan pending.')
  })

  it('reports the count, the date, and that nobody reviewed it after a scan', () => {
    expect(statusLine(scanned())).toBe(
      '2 tools detected on 2026-09-08. Domain control verified. Not reviewed by Sightmap.'
    )
  })

  it('says nothing about the site being safe, trusted, or approved', () => {
    const html = renderTryCard(scanned())
    expect(/\b(safe|trusted|approved|endorsed)\b/i.test(html)).toBe(false)
  })
})

describe('promptFor', () => {
  it('builds the prompt from the first read tool', () => {
    expect(promptFor(scanned())).toBe(
      'Open https://example.dev and call search_docs — Search the documentation.'
    )
  })

  it('offers no prompt when the scan found no read tool', () => {
    const only = scanned()
    only.scan!.tools = only.scan!.tools.filter((t) => t.kind !== 'read')
    expect(promptFor(only)).toBeNull()
    expect(renderTryCard(only)).not.toContain('Try it')
  })

  it('offers no prompt before a scan', () => {
    expect(promptFor(record())).toBeNull()
  })
})

describe('shareText', () => {
  // The audience is named as a capability ("WebMCP agents"), never as a
  // product: whichever agent a reader points at the site is their business,
  // and naming one would read as an endorsement in both directions.
  it('names WebMCP agents and only the host, the count, and the card', () => {
    expect(shareText(scanned(), 'https://sightmap.org/try/example.dev')).toBe(
      '2 WebMCP tools detected on example.dev, callable by WebMCP agents. https://sightmap.org/try/example.dev'
    )
  })

  it('still names WebMCP agents before a scan', () => {
    const text = shareText(record(), 'https://sightmap.org/try/example.dev')
    expect(text).toContain('WebMCP agents')
    expect(text).toBe('example.dev is claimed on Sightmap for WebMCP agents. https://sightmap.org/try/example.dev')
  })

  it('carries the same text into the share link', () => {
    const html = renderTryCard(scanned())
    const share = anchors(html).find((a) => a.href.startsWith('https://twitter.com/intent/tweet'))!
    const text = new URL(share.href.replace(/&amp;/g, '&')).searchParams.get('text')
    expect(text).toBe(shareText(scanned(), 'https://sightmap.org/try/example.dev'))
  })
})

describe('renderTryCard', () => {
  it('leads with the destination origin and repeats it beside every outbound link', () => {
    const html = renderTryCard(scanned())
    const body = html.slice(html.indexOf('<main>'))
    expect(body.indexOf('https://example.dev')).toBe(body.indexOf('<h1>') + '<h1>'.length)

    const outbound = anchors(body).filter((a) => !a.href.startsWith('/'))
    expect(outbound.length).toBeGreaterThanOrEqual(3)
    // One mention for the heading, one for each link that leaves the site.
    const mentions = body.split('https://example.dev').length - 1
    expect(mentions).toBeGreaterThanOrEqual(outbound.length + 1)
  })

  it('marks every link that leaves sightmap.org nofollow noopener', () => {
    const html = renderTryCard(scanned())
    const outbound = anchors(html).filter((a) => !a.href.startsWith('/'))
    expect(outbound.map((a) => a.rel)).toEqual(outbound.map(() => 'nofollow noopener'))
    expect(outbound.some((a) => a.href.startsWith('mailto:atlas@sightmap.org?subject=Report%20'))).toBe(true)
  })

  it('renders a hostile tool description as text, in the body and in attributes', () => {
    const hostile = scanned()
    hostile.scan!.tools[0] = {
      name: '"><script>alert(1)</script>',
      description: `"><img src=x onerror="alert(1)"> & 'quoted'`,
      kind: 'read',
      page: '/"><b>',
    }
    const html = renderTryCard(hostile)

    expect(html).not.toContain('<script>alert(1)</script>')
    expect(html).not.toContain('<img src=x')
    expect(html).not.toContain('onerror="alert(1)"')
    expect(html).toContain('&lt;img src=x onerror=&quot;alert(1)&quot;&gt;')
    expect(html).toContain('&amp; &#39;quoted&#39;')

    // The same text also reaches the prompt block and the share link; neither
    // may reopen an attribute.
    const share = anchors(html).find((a) => a.href.startsWith('https://twitter.com'))!
    expect(share.href).not.toContain('<')
    expect(html).toContain('<pre>Open https://example.dev and call &quot;&gt;&lt;script&gt;')
  })

  it('escapes a hostile host everywhere it appears, including the mailto subject', () => {
    const html = renderTryCard(record({ host: 'a"b<c.example' }))
    expect(html).not.toContain('a"b<c.example')
    expect(html).toContain('https://a&quot;b&lt;c.example')
    expect(html).toContain('subject=Report%20a%22b%3Cc.example')
  })

  it('asks crawlers to skip it and points at a static social image', () => {
    const html = renderTryCard(scanned())
    expect(html).toContain('<meta name="robots" content="noindex,nofollow">')
    expect(html).toContain('<meta property="og:image" content="https://sightmap.org/try-og.png">')
    expect(html).toContain('<title>example.dev · Sightmap try</title>')
  })

  it('shows the unreviewed tag and the one-line pointer to the Atlas', () => {
    const html = renderTryCard(scanned())
    expect(html).toContain('>Unreviewed<')
    expect(html).toContain('A site appears in the Sightmap Atlas only after a maintainer reviews it.')
    expect(html).toContain('href="/atlas"')
  })

  it('lists only the tools in the record, with their kind labels', () => {
    const html = renderTryCard(scanned())
    expect(html).toContain('search_docs')
    expect(html).toContain('>Read<')
    expect(html).toContain('add_to_cart')
    expect(html).toContain('>Action<')
    expect(html).toContain('/docs')
  })

  it('lists no tools before a scan', () => {
    const html = renderTryCard(record())
    expect(html).not.toContain('Tools the scanner saw')
    expect(html).toContain('Scan pending.')
  })

  it('renders the origin from the record rather than fetching it', () => {
    // A card must never be built from a live request to the submitted site;
    // the record is the only source, so `url` beyond the origin stays unused.
    const html = renderTryCard(record({ url: 'https://example.dev/deep/path?token=secret' }))
    expect(html).not.toContain('token=secret')
  })

  it('takes the site origin and report address from options', () => {
    const html = renderTryCard(record(), {
      siteUrl: 'https://deploy-preview--sightmap.netlify.app/',
      reportEmail: 'someone@example.org',
    })
    expect(html).toContain('content="https://deploy-preview--sightmap.netlify.app/try-og.png"')
    expect(html).toContain('mailto:someone@example.org?subject=Report%20example.dev')
  })
})

describe('renderGone and renderNotFound', () => {
  it('say only that the card is gone or absent, and stay unindexed', () => {
    for (const html of [renderGone(), renderNotFound()]) {
      expect(html).toContain('<meta name="robots" content="noindex,nofollow">')
      expect(anchors(html).every((a) => a.href.startsWith('/'))).toBe(true)
    }
    expect(renderGone()).toContain('This card was removed.')
    expect(renderNotFound()).toContain('There is no card at this address.')
  })
})

describe('decideTryCard', () => {
  const base = {
    preflightOk: true,
    listedSlug: null,
    quarantined: false,
    record: null,
    now: Date.parse('2026-09-10T00:00:00.000Z'),
  }

  it('404s a host that fails the shape rules before looking anything up', () => {
    expect(decideTryCard({ ...base, preflightOk: false, record: record() })).toEqual({ kind: 'not-found' })
  })

  it('sends an admitted host to its listing', () => {
    expect(decideTryCard({ ...base, listedSlug: 'example-dev', record: record() })).toEqual({
      kind: 'listed',
      slug: 'example-dev',
    })
  })

  it('prefers the listing over a quarantine or a record', () => {
    expect(
      decideTryCard({ ...base, listedSlug: 'example-dev', quarantined: true, record: record() })
    ).toEqual({ kind: 'listed', slug: 'example-dev' })
  })

  it('reports a quarantined host as gone, with or without a record', () => {
    expect(decideTryCard({ ...base, quarantined: true })).toEqual({ kind: 'gone' })
    expect(decideTryCard({ ...base, quarantined: true, record: record() })).toEqual({ kind: 'gone' })
  })

  it('404s when there is no record', () => {
    expect(decideTryCard(base)).toEqual({ kind: 'not-found' })
  })

  it('404s an expired record rather than rendering a stale card', () => {
    const stale = record({ expiresAt: '2026-09-09T23:59:59.000Z' })
    expect(decideTryCard({ ...base, record: stale })).toEqual({ kind: 'not-found' })
    // The boundary is inclusive: a record expiring exactly now is expired.
    const onTheDot = record({ expiresAt: '2026-09-10T00:00:00.000Z' })
    expect(decideTryCard({ ...base, record: onTheDot })).toEqual({ kind: 'not-found' })
  })

  it('renders a live record', () => {
    const live = record()
    expect(decideTryCard({ ...base, record: live })).toEqual({ kind: 'card', record: live })
  })
})
