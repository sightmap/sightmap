import { describe, expect, it } from 'vitest'
import {
  figLabel,
  filterDirectory,
  filterEntries,
  markColor,
  markInitial,
  matchesListingQuery,
  primaryDomain,
  statParts,
} from './atlas'
import type { AtlasEntry } from '@/types/atlas'
import type { DirectoryListing, ScanReport } from '@/types/directory'

const base: AtlasEntry = {
  slug: 'alpha-site',
  name: 'Alpha',
  site_url: 'https://alpha.example/docs',
  domains: ['alpha.example'],
  description: 'A documentation site.',
  categories: ['docs'],
  author: 'chiplay',
  created: '2026-01-01',
  updated: '2026-01-01',
  last_verified: '2026-01-01',
  cli_version: '0.17.0',
  spec_version: 1,
  method: 'browser',
  auth: 'none',
  stats: { views: 1, components: 1, requests: 1, properties: 0, memory: 0 },
  per_view: [],
  screenshots: [],
  files: [],
  screenshotUrls: [],
  bodyHtml: '',
}

const entry = (over: Partial<AtlasEntry>): AtlasEntry => ({ ...base, ...over })

describe('primaryDomain', () => {
  it('prefers the schema domains list', () => {
    expect(primaryDomain(entry({ domains: ['a.example', 'b.example'] }))).toBe('a.example')
  })

  it('falls back to the site_url host when no domains are listed', () => {
    expect(primaryDomain(entry({ domains: [] }))).toBe('alpha.example')
  })

  it('falls back to the raw site_url rather than returning empty', () => {
    expect(primaryDomain({ domains: [], site_url: 'not a url' })).toBe('not a url')
  })
})

describe('mark', () => {
  it('uses the domain first alphanumeric, uppercased', () => {
    expect(markInitial('alpha.example')).toBe('A')
    expect(markInitial('9lives.dev')).toBe('9')
    expect(markInitial('-.-')).toBe('?')
  })

  it('is stable for a given domain and varies across domains', () => {
    expect(markColor('alpha.example')).toBe(markColor('alpha.example'))
    const colors = new Set(['a.dev', 'b.dev', 'c.dev', 'd.dev', 'e.dev'].map(markColor))
    expect(colors.size).toBeGreaterThan(1)
  })
})

describe('statParts', () => {
  it('singularizes a count of one', () => {
    const parts = statParts(entry({ stats: { ...base.stats, views: 1, components: 1, requests: 1 } }))
    expect(parts.map((p) => p.label)).toEqual(['view', 'component', 'request'])
  })

  it('pluralizes everything else, including zero', () => {
    const parts = statParts(entry({ stats: { ...base.stats, views: 4, components: 28, requests: 0 } }))
    expect(parts.map((p) => `${p.value} ${p.label}`)).toEqual(['4 views', '28 components', '0 requests'])
  })
})

describe('figLabel', () => {
  it('is a zero-padded, 1-based FIG number', () => {
    expect(figLabel(0)).toBe('FIG. 01')
    expect(figLabel(11)).toBe('FIG. 12')
  })
})

describe('filterEntries', () => {
  const entries = [
    entry({ slug: 'a', name: 'Alpha', categories: ['docs'], description: 'A documentation site.' }),
    entry({
      slug: 'b',
      name: 'Beta',
      categories: ['shop'],
      description: 'A storefront.',
      author: 'someone',
      domains: ['beta.test'],
    }),
    entry({ slug: 'c', name: 'Gamma', categories: ['docs', 'shop'], domains: ['gamma.test'] }),
  ]

  it('returns everything when nothing is set', () => {
    expect(filterEntries(entries, '', '')).toHaveLength(3)
  })

  it('filters by category', () => {
    expect(filterEntries(entries, 'shop', '').map((e) => e.slug)).toEqual(['b', 'c'])
  })

  it('searches name, description, author, domains and categories', () => {
    expect(filterEntries(entries, '', 'storefront').map((e) => e.slug)).toEqual(['b'])
    expect(filterEntries(entries, '', 'someone').map((e) => e.slug)).toEqual(['b'])
    expect(filterEntries(entries, '', 'gamma.test').map((e) => e.slug)).toEqual(['c'])
    expect(filterEntries(entries, '', 'docs').map((e) => e.slug)).toEqual(['a', 'c'])
  })

  it('searches the slug, which is what someone pastes back off a card', () => {
    // `sightmap atlas find` searches the slug against this same index.json. A
    // slug the CLI resolves and the grid does not is one query with two
    // answers, so the slug has to be in the haystack here too.
    const bySlug = [
      entry({ slug: 'square-pos', name: 'Square', description: 'A card reader.' }),
      entry({ slug: 'other-site', name: 'Other', description: 'Something else.' }),
    ]
    expect(filterEntries(bySlug, '', 'square-pos').map((e) => e.slug)).toEqual(['square-pos'])
    expect(filterEntries(bySlug, '', 'SQUARE-POS').map((e) => e.slug)).toEqual(['square-pos'])
  })

  it('ignores case and surrounding whitespace', () => {
    expect(filterEntries(entries, '', '  ALPHA ').map((e) => e.slug)).toEqual(['a'])
  })

  it('applies category and query together', () => {
    expect(filterEntries(entries, 'docs', 'gamma').map((e) => e.slug)).toEqual(['c'])
    expect(filterEntries(entries, 'shop', 'alpha')).toEqual([])
  })
})

// The directory half of the gallery. A listing is not an entry — one category
// rather than many, a host rather than a domains list, and tool names, which
// are the one thing a listing has that an entry does not.
const emptyReport: ScanReport = {
  version: 1,
  url: 'https://alpha.example/',
  finalUrl: 'https://alpha.example/',
  host: 'alpha.example',
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
}

const baseListing: DirectoryListing = {
  slug: 'alpha-tools',
  name: 'Alpha Tools',
  url: 'https://alpha.example/',
  host: 'alpha.example',
  description: 'A fixture site with three WebMCP tools.',
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
  report: emptyReport,
  scanDates: ['2026-09-09'],
  drift: null,
  counts: emptyReport.counts,
  scannedAt: emptyReport.scannedAt,
  surface: 'native',
  status: 'tools-found',
}

const listing = (over: Partial<DirectoryListing>): DirectoryListing => ({ ...baseListing, ...over })

describe('matchesListingQuery', () => {
  it('matches an empty query', () => {
    expect(matchesListingQuery(baseListing, '')).toBe(true)
    expect(matchesListingQuery(baseListing, '   ')).toBe(true)
  })

  it('searches slug, name, host, description, category and type', () => {
    expect(matchesListingQuery(baseListing, 'alpha-tools')).toBe(true)
    expect(matchesListingQuery(baseListing, 'Alpha Tools')).toBe(true)
    expect(matchesListingQuery(baseListing, 'alpha.example')).toBe(true)
    expect(matchesListingQuery(baseListing, 'fixture')).toBe(true)
    expect(matchesListingQuery(baseListing, 'devtools')).toBe(true)
    expect(matchesListingQuery(baseListing, 'live')).toBe(true)
    expect(matchesListingQuery(baseListing, 'nothing here')).toBe(false)
  })

  it('searches tool names, which is what someone hunting a callable site types', () => {
    const l = listing({
      tools: [
        { name: 'search_flights', kind: 'read', description: 'Search flights.', page: '/' },
        { name: 'hold_seat', kind: 'action', description: 'Hold a seat.', page: '/book' },
      ],
    })
    expect(matchesListingQuery(l, 'search_flights')).toBe(true)
    expect(matchesListingQuery(l, 'hold_seat')).toBe(true)
    expect(matchesListingQuery(l, 'cancel_booking')).toBe(false)
  })

  it('searches labels and collections', () => {
    const l = listing({ labels: ['featured'], collections: ['competition-demos'] })
    expect(matchesListingQuery(l, 'featured')).toBe(true)
    expect(matchesListingQuery(l, 'competition-demos')).toBe(true)
  })

  it('ignores case and surrounding whitespace, like the entry search', () => {
    expect(matchesListingQuery(baseListing, '  ALPHA-TOOLS ')).toBe(true)
  })
})

describe('filterDirectory', () => {
  // Distinct hosts as well as names: the host is in the haystack, so three
  // listings sharing one host would make every text query match all three.
  const listings = [
    listing({ slug: 'a', name: 'Alpha', host: 'alpha.example', category: 'devtools' }),
    listing({
      slug: 'b',
      name: 'Beta',
      host: 'beta.example',
      category: 'commerce',
      description: 'A storefront.',
    }),
    listing({ slug: 'c', name: 'Gamma', host: 'gamma.example', category: 'devtools', type: 'demo' }),
  ]

  it('returns everything when nothing is set', () => {
    expect(filterDirectory(listings, '', '')).toHaveLength(3)
  })

  it('filters by the single category, not by a list', () => {
    expect(filterDirectory(listings, 'devtools', '').map((l) => l.slug)).toEqual(['a', 'c'])
    expect(filterDirectory(listings, 'commerce', '').map((l) => l.slug)).toEqual(['b'])
  })

  it('applies category and query together', () => {
    expect(filterDirectory(listings, 'devtools', 'gamma').map((l) => l.slug)).toEqual(['c'])
    expect(filterDirectory(listings, 'commerce', 'alpha')).toEqual([])
  })

  it('does not filter by type — the type chips are the page\'s own cut', () => {
    // filterDirectory answers "category + text"; ?type= is applied by
    // AtlasIndex on top of it, so a demo listing survives here.
    expect(filterDirectory(listings, '', 'gamma').map((l) => l.slug)).toEqual(['c'])
  })
})
