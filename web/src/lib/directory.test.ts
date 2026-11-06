import { describe, expect, it } from 'vitest'
import {
  badgeMarkdown,
  checksPassed,
  formatScanned,
  kindClass,
  kindLabel,
  shareUrls,
  toolsByPage,
} from './directory'
import type { DirectoryListing, ScanPage, ScanReport, ScanTool } from '@/types/directory'

const tool = (over: Partial<ScanTool> & { name: string }): ScanTool => ({
  description: '',
  inputSchema: { type: 'object' },
  page: '/',
  pages: ['/'],
  impl: 'imperative',
  api: 'navigator',
  risk: 'read',
  riskReason: '',
  warnings: [],
  ...over,
})

const page = (path: string, tools: string[] = []): ScanPage => ({
  url: `https://alpha.example.org${path}`,
  path,
  title: path,
  status: 200,
  surface: 'native',
  tools,
})

const report = (over: Partial<ScanReport> = {}): ScanReport => ({
  version: 1,
  url: 'https://alpha.example.org/',
  finalUrl: 'https://alpha.example.org/',
  host: 'alpha.example.org',
  scannedAt: '2026-09-09T20:00:00.000Z',
  scanner: { name: 'sightmap-atlas-scan', version: '1.0.0', browser: 'sightmap 0.31.2' },
  surface: 'native',
  status: 'tools-found',
  intent: null,
  pages: [],
  tools: [],
  checks: [],
  counts: {
    tools: 0,
    pages: 0,
    read: 0,
    action: 0,
    sensitive: 0,
    described: 0,
    withInputSchema: 0,
    declarative: 0,
  },
  hints: { forms: [], links: [], title: '', description: '' },
  notes: [],
  ...over,
})

const listing = (over: Partial<DirectoryListing> = {}): DirectoryListing => {
  const rep = over.report ?? report()
  return {
    slug: 'alpha-tools',
    name: 'Alpha Tools',
    url: 'https://alpha.example.org/',
    host: 'alpha.example.org',
    description: 'A fixture site.',
    category: 'devtools',
    type: 'live',
    built_with_sightkick: false,
    submitted_by: 'owner',
    labels: [],
    collections: [],
    added: '2026-09-09',
    updated: '2026-09-09',
    scan: 'scans/alpha-tools/2026-09-09.json',
    tools: [],
    suggested_journeys: [],
    strengths: [],
    improvements: [],
    scanDates: ['2026-09-09'],
    drift: null,
    scannedAt: rep.scannedAt,
    surface: rep.surface,
    status: rep.status,
    counts: rep.counts,
    ...over,
    report: rep,
  }
}

describe('kindLabel / kindClass', () => {
  it('labels the three kinds in sentence case', () => {
    expect([kindLabel('read'), kindLabel('action'), kindLabel('sensitive')]).toEqual([
      'Read',
      'Action',
      'Sensitive',
    ])
  })

  it('gives each kind its own modifier so the three colours can differ', () => {
    expect(kindClass('read')).toBe('atlas-kind atlas-kind--read')
    expect(kindClass('action')).toBe('atlas-kind atlas-kind--action')
    expect(kindClass('sensitive')).toBe('atlas-kind atlas-kind--sensitive')
    // Distinct, not ranked: no shared "worse" modifier between them.
    expect(new Set([kindClass('read'), kindClass('action'), kindClass('sensitive')]).size).toBe(3)
  })
})

describe('toolsByPage', () => {
  it('groups tools by the page they were first seen on, in scan order', () => {
    const l = listing({
      report: report({
        pages: [page('/'), page('/search'), page('/account')],
        tools: [
          tool({ name: 'get_page' }),
          tool({ name: 'search', page: '/search' }),
          tool({ name: 'sign_out', page: '/account', risk: 'sensitive' }),
          tool({ name: 'get_title' }),
        ],
      }),
    })
    expect(toolsByPage(l).map((g) => [g.page, g.tools.map((t) => t.name)])).toEqual([
      ['/', ['get_page', 'get_title']],
      ['/search', ['search']],
      ['/account', ['sign_out']],
    ])
  })

  it('drops a visited page that registered nothing', () => {
    const l = listing({
      report: report({ pages: [page('/'), page('/about')], tools: [tool({ name: 'get_page' })] }),
    })
    expect(toolsByPage(l).map((g) => g.page)).toEqual(['/'])
  })

  it('keeps a tool whose page was never listed rather than dropping it', () => {
    // A redirect can leave a tool's `page` off the visited list; silently
    // losing it would understate the surface.
    const l = listing({
      report: report({ pages: [page('/')], tools: [tool({ name: 'checkout', page: '/cart' })] }),
    })
    expect(toolsByPage(l).map((g) => g.page)).toEqual(['/cart'])
  })

  it('returns nothing when the scan found no tools', () => {
    expect(toolsByPage(listing({ report: report({ pages: [page('/')] }) }))).toEqual([])
  })
})

describe('checksPassed', () => {
  it('counts passing checks out of the total', () => {
    const l = listing({
      report: report({
        checks: [
          { id: 'a', label: 'A', ok: true, detail: '' },
          { id: 'b', label: 'B', ok: false, detail: '' },
          { id: 'c', label: 'C', ok: true, detail: '' },
        ],
      }),
    })
    expect(checksPassed(l)).toEqual({ passed: 2, total: 3 })
  })

  it('is 0/0 rather than a divide when there are no checks', () => {
    expect(checksPassed(listing())).toEqual({ passed: 0, total: 0 })
  })
})

describe('shareUrls', () => {
  const l = listing({ counts: { ...report().counts, tools: 3 } })

  it('points every target at the canonical listing URL', () => {
    expect(shareUrls(l).url).toBe('https://sightmap.org/atlas/alpha-tools')
    expect(shareUrls(l).x).toContain(encodeURIComponent('https://sightmap.org/atlas/alpha-tools'))
    expect(shareUrls(l).linkedin).toBe(
      'https://www.linkedin.com/sharing/share-offsite/?url=' +
        encodeURIComponent('https://sightmap.org/atlas/alpha-tools')
    )
  })

  it('states a count and nothing else — no "verified", no "safe"', () => {
    const text = decodeURIComponent(shareUrls(l).x)
    expect(text).toContain('3 callable WebMCP tools')
    expect(text).toContain('scanned by Sightmap Atlas')
    expect(text).not.toMatch(/verified|certified|safe|trusted/i)
  })

  it('singularizes a one-tool listing', () => {
    const one = listing({ counts: { ...report().counts, tools: 1 } })
    expect(decodeURIComponent(shareUrls(one).x)).toContain('1 callable WebMCP tool,')
  })
})

describe('badgeMarkdown', () => {
  it('links the badge image back to the listing it came from', () => {
    expect(badgeMarkdown('alpha-tools', 'Alpha Tools')).toBe(
      '[![Alpha Tools on Sightmap Atlas](https://sightmap.org/atlas/alpha-tools/badge.svg)]' +
        '(https://sightmap.org/atlas/alpha-tools)'
    )
  })

  it('strips brackets from the name, which would close the markdown alt text early', () => {
    expect(badgeMarkdown('x', 'A [beta] app')).toContain('[![A beta app on Sightmap Atlas]')
  })
})

describe('formatScanned', () => {
  it('formats a full ISO instant', () => {
    expect(formatScanned('2026-09-09T20:00:00.000Z')).toMatch(/September 9, 2026|September 10, 2026/)
  })

  it('pins a date-only string to local midnight, so it never renders a day early', () => {
    // `new Date('2026-09-09')` is UTC midnight and prints as the 8th west of
    // Greenwich; the T00:00:00 suffix is what stops that.
    expect(formatScanned('2026-09-09')).toBe('September 9, 2026')
  })

  it('passes an unparseable value through instead of printing "Invalid Date"', () => {
    expect(formatScanned('not a date')).toBe('not a date')
    expect(formatScanned('')).toBe('')
  })
})
