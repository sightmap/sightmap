import path from 'node:path'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { loadDirectory } from './directory'
import {
  directoryIndexDocument,
  directoryIndexListing,
  directoryStatsDocument,
  hostDocuments,
  isSafeHost,
  listingUrls,
  reportWithoutTranscript,
  siteDocument,
  toolsDocument,
} from './directory-outputs'
import type { DirectoryListing } from '../../src/types/directory'

const FIXTURES = path.resolve(__dirname, '__fixtures__/directory')

let warn: ReturnType<typeof vi.spyOn>
beforeEach(() => {
  warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
})
afterEach(() => {
  warn.mockRestore()
})

function alpha(): DirectoryListing {
  const { listings } = loadDirectory(FIXTURES)
  const found = listings.find((l) => l.slug === 'alpha-tools')
  if (!found) throw new Error('fixture listing alpha-tools did not load')
  return found
}

describe('listingUrls', () => {
  it('builds every machine URL from SITE_URL and the slug', () => {
    expect(listingUrls('alpha-tools')).toEqual({
      html: 'https://sightmap.org/atlas/alpha-tools',
      markdown: 'https://sightmap.org/atlas/alpha-tools.md',
      json: 'https://sightmap.org/atlas/sites/alpha-tools.json',
      tools: 'https://sightmap.org/atlas/sites/alpha-tools/tools.json',
      scan: 'https://sightmap.org/atlas/scans/alpha-tools.json',
      badge: 'https://sightmap.org/atlas/alpha-tools/badge.svg',
    })
  })
})

describe('directoryIndexListing', () => {
  it('summarises a listing without carrying any tool schema', () => {
    const entry = directoryIndexListing(alpha())
    expect(entry).toEqual({
      slug: 'alpha-tools',
      name: 'Alpha Tools',
      url: 'https://alpha.example.org/',
      host: 'alpha.example.org',
      description: 'A fixture site with three WebMCP tools.',
      category: 'devtools',
      type: 'live',
      built_with_sightkick: true,
      labels: ['promising'],
      collections: ['built-with-sightkick'],
      surface: 'polyfilled',
      status: 'tools-found',
      tool_count: 2,
      tool_kinds: { read: 1, action: 1, sensitive: 0 },
      pages_scanned: 2,
      last_scanned: '2026-09-08',
      added: '2026-09-01',
      updated: '2026-09-08',
      html: 'https://sightmap.org/atlas/alpha-tools',
      json: 'https://sightmap.org/atlas/sites/alpha-tools.json',
      tools: 'https://sightmap.org/atlas/sites/alpha-tools/tools.json',
      scan: 'https://sightmap.org/atlas/scans/alpha-tools.json',
      badge: 'https://sightmap.org/atlas/alpha-tools/badge.svg',
    })
    // The index is the file an agent reads whole, so the payload that makes a
    // listing large has to stay out of it.
    expect(JSON.stringify(entry)).not.toContain('inputSchema')
  })
})

describe('directoryIndexDocument', () => {
  it('wraps the listings in a versioned, stamped envelope', () => {
    const doc = directoryIndexDocument([alpha()], '2026-09-09T00:00:00.000Z')
    expect(doc.schema_version).toBe(1)
    expect(doc.generated_at).toBe('2026-09-09T00:00:00.000Z')
    expect(doc.listings.map((l) => l.slug)).toEqual(['alpha-tools'])
  })

  it('is a valid, empty document when nothing is listed', () => {
    const doc = directoryIndexDocument([], '2026-09-09T00:00:00.000Z')
    expect(doc.listings).toEqual([])
    expect(JSON.stringify(doc)).not.toContain('undefined')
  })
})

describe('directoryStatsDocument', () => {
  it('counts listings, tools, surfaces and categories', () => {
    const stats = directoryStatsDocument([alpha()], {
      generatedAt: '2026-09-09T00:00:00.000Z',
      communityMaps: 12,
    })
    expect(stats).toEqual({
      schema_version: 1,
      generated_at: '2026-09-09T00:00:00.000Z',
      listings: { live: 1, demo: 0, total: 1 },
      community_maps: 12,
      tools: { total: 2, read: 1, action: 1, sensitive: 0, declarative: 0 },
      surfaces: { native: 0, polyfilled: 1, declarative: 0, absent: 0 },
      categories: { devtools: 1 },
      built_with_sightkick: 1,
    })
  })

  it('reports zeroes for every surface when there is nothing listed', () => {
    const stats = directoryStatsDocument([], { generatedAt: 'now', communityMaps: 0 })
    expect(stats.listings).toEqual({ live: 0, demo: 0, total: 0 })
    expect(stats.surfaces).toEqual({ native: 0, polyfilled: 0, declarative: 0, absent: 0 })
    expect(stats.categories).toEqual({})
  })

  it('sorts category ids so a reordered rebuild produces the same file', () => {
    const one = alpha()
    const two: DirectoryListing = { ...one, slug: 'beta', category: 'commerce', type: 'demo' }
    const stats = directoryStatsDocument([one, two], { generatedAt: 'now', communityMaps: 0 })
    expect(Object.keys(stats.categories)).toEqual(['commerce', 'devtools'])
    expect(stats.listings).toEqual({ live: 1, demo: 1, total: 2 })
  })
})

describe('siteDocument', () => {
  it('carries the reviewed listing, its checks, drift and history — but not the report', () => {
    const doc = siteDocument(alpha())
    expect(doc.slug).toBe('alpha-tools')
    expect(doc.scanned_at).toBe('2026-09-08T10:00:00.000Z')
    expect(doc.last_scanned).toBe('2026-09-08')
    expect(doc.counts.tools).toBe(2)
    expect(doc.tool_kinds).toEqual({ read: 1, action: 1, sensitive: 0 })
    expect(doc.tools.map((t) => t.name)).toEqual(['search', 'open_page'])
    expect(doc.checks.map((c) => c.id)).toEqual(['named', 'route-scoped'])
    expect(doc.drift).toEqual({ since: '2026-09-01', added: ['open_page'], removed: ['legacy'] })
    expect(doc.scan_dates).toEqual(['2026-09-08', '2026-09-01'])
    expect(doc.journey).toBeNull()
    expect('report' in doc).toBe(false)
    expect(JSON.stringify(doc)).not.toContain('inputSchema')
  })

  it('links every machine twin, including one URL per scan on file', () => {
    const doc = siteDocument(alpha())
    expect(doc.links.markdown).toBe('https://sightmap.org/atlas/alpha-tools.md')
    expect(doc.links.badge).toBe('https://sightmap.org/atlas/alpha-tools/badge.svg')
    expect(doc.links.directory).toBe('https://sightmap.org/atlas/directory.json')
    expect(doc.links.stats).toBe('https://sightmap.org/atlas/stats.json')
    expect(doc.links.scans).toEqual([
      'https://sightmap.org/atlas/scans/alpha-tools/2026-09-08.json',
      'https://sightmap.org/atlas/scans/alpha-tools/2026-09-01.json',
    ])
  })
})

describe('toolsDocument', () => {
  it('publishes every scanned tool with its input schema intact', () => {
    const doc = toolsDocument(alpha())
    expect(doc.slug).toBe('alpha-tools')
    expect(doc.scanned_at).toBe('2026-09-08T10:00:00.000Z')
    expect(doc.tools.map((t) => t.name)).toEqual(['search', 'open_page'])
    expect(doc.tools[0].inputSchema).toEqual({
      type: 'object',
      properties: { q: { type: 'string' } },
      required: ['q'],
    })
    expect(doc.tools[0].risk).toBe('read')
  })
})

describe('hostDocuments', () => {
  it('publishes the bare host and its www. spelling', () => {
    const files = hostDocuments(alpha())
    expect(files.map((f) => f.host)).toEqual(['alpha.example.org', 'www.alpha.example.org'])
    expect(files[0].document).toEqual({
      slug: 'alpha-tools',
      url: 'https://alpha.example.org/',
      tool_count: 2,
      last_scanned: '2026-09-08',
      html: 'https://sightmap.org/atlas/alpha-tools',
      json: 'https://sightmap.org/atlas/sites/alpha-tools.json',
    })
    // Both spellings resolve to the same listing.
    expect(files[1].document).toEqual(files[0].document)
  })

  it('normalises a host that is already www. rather than doubling the prefix', () => {
    const listing: DirectoryListing = { ...alpha(), host: 'WWW.Example.COM' }
    expect(hostDocuments(listing).map((f) => f.host)).toEqual(['example.com', 'www.example.com'])
  })

  it('publishes no lookup file for a host that is not a safe file name', () => {
    const listing: DirectoryListing = { ...alpha(), host: '../../etc/passwd' }
    expect(hostDocuments(listing)).toEqual([])
    expect(isSafeHost('example..com')).toBe(false)
    expect(isSafeHost('example.com')).toBe(true)
  })
})

describe('reportWithoutTranscript', () => {
  it('drops the CLI transcript lines and keeps every other note', () => {
    const report = alpha().report
    const withNotes = { ...report, notes: ['$ sightmap browser start', 'Ran headless.'] }
    expect(reportWithoutTranscript(withNotes).notes).toEqual(['Ran headless.'])
  })

  it('returns the same object when there is nothing to strip', () => {
    const report = alpha().report
    expect(reportWithoutTranscript(report)).toBe(report)
  })
})
