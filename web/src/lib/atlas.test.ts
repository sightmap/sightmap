import { describe, expect, it } from 'vitest'
import { figLabel, filterEntries, markColor, markInitial, primaryDomain, statParts } from './atlas'
import type { AtlasEntry } from '@/types/atlas'

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
