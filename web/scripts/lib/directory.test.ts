import path from 'node:path'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import YAML from 'yaml'
import { ListingSchema, listingToYaml, loadDirectory, slugFromHost, uniqueSlug } from './directory'
import { listingMarkdown, scanReportMarkdown } from './directory-markdown'
import { sightkickStarter } from './sightkick-starter'

const FIXTURES = path.resolve(__dirname, '__fixtures__/directory')

let warn: ReturnType<typeof vi.spyOn>
beforeEach(() => {
  warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
})
afterEach(() => {
  warn.mockRestore()
})

describe('loadDirectory', () => {
  it('loads a valid listing with its scan, history, and drift', () => {
    const { listings, skipped } = loadDirectory(FIXTURES)
    expect(listings.map((l) => l.slug)).toEqual(['alpha-tools'])
    expect(skipped.sort()).toEqual(['broken', 'no-scan'])

    const alpha = listings[0]
    expect(alpha.counts.tools).toBe(2)
    expect(alpha.surface).toBe('polyfilled')
    expect(alpha.scannedAt).toBe('2026-09-08T10:00:00.000Z')
    expect(alpha.scanDates).toEqual(['2026-09-08', '2026-09-01'])
    expect(alpha.drift).toEqual({ since: '2026-09-01', added: ['open_page'], removed: ['legacy'] })
    expect(alpha.labels).toEqual(['promising'])
    expect(alpha.built_with_sightkick).toBe(true)
  })

  it('warns instead of throwing for the broken and scan-less listings', () => {
    loadDirectory(FIXTURES)
    const messages = warn.mock.calls.map((c) => String(c[0]))
    expect(messages.some((m) => m.includes('skipping directory listing broken'))).toBe(true)
    expect(messages.some((m) => m.includes('skipping directory listing no-scan'))).toBe(true)
  })

  it('drops a listing whose slug the community atlas already uses', () => {
    const { listings, skipped } = loadDirectory(FIXTURES, ['alpha-tools'])
    expect(listings).toEqual([])
    expect(skipped).toContain('alpha-tools')
  })

  it('returns nothing for a missing data dir', () => {
    expect(loadDirectory(path.join(FIXTURES, 'nope'))).toEqual({ listings: [], skipped: [] })
  })
})

describe('slug helpers', () => {
  it('derives a slug from a host and avoids collisions', () => {
    expect(slugFromHost('www.Example.co.uk')).toBe('example-co-uk')
    expect(uniqueSlug('example', new Set(['example', 'example-2']))).toBe('example-3')
  })
})

describe('listingToYaml', () => {
  it('round-trips through the schema in the schema key order', () => {
    const { listings } = loadDirectory(FIXTURES)
    const { report: _r, scanDates: _d, drift: _x, counts: _c, scannedAt: _s, surface: _u, status: _t, ...meta } = listings[0]
    const yaml = listingToYaml(meta)
    expect(yaml.startsWith('slug: alpha-tools\nname: Alpha Tools\n')).toBe(true)
    expect(ListingSchema.parse(YAML.parse(yaml))).toEqual(meta)
  })
})

describe('markdown twins', () => {
  it('renders tool names inside code spans and escapes markdown in descriptions', () => {
    const { listings } = loadDirectory(FIXTURES)
    const l = listings[0]
    l.tools[0].description = 'Search *the* [docs](javascript:alert(1)) <b>now</b>'
    const md = listingMarkdown(l)
    expect(md).toContain('`search`')
    expect(md).not.toContain('[docs](javascript:alert(1))')
    expect(md).toContain('\\<b\\>now\\</b\\>')
    expect(md).toContain('## Since 2026-09-01')
    expect(md).toContain('Added: `open_page`')
  })

  it('renders a scan report with checks, tools, and pages', () => {
    const { listings } = loadDirectory(FIXTURES)
    const md = scanReportMarkdown(listings[0].report)
    expect(md).toContain('# Atlas scan: alpha.example.org')
    expect(md).toContain('- Tools discovered: 2 (1 read, 1 action, 0 sensitive; 0 declarative)')
    expect(md).toContain('- ✓ Tool set changes with the page (2 distinct tool sets across 2 pages)')
    expect(md).toContain('- `/docs` — polyfilled, 1 tool(s)')
  })
})

describe('sightkickStarter', () => {
  it('offers a verification transcript when tools were found', () => {
    const { listings } = loadDirectory(FIXTURES)
    const s = sightkickStarter(listings[0].report)
    expect(s.kind).toBe('verify')
    expect(s.code).toContain('sightmap browser mcp call search --param q="<q>"')
    expect(s.prompt).toContain('sightmap browser start --url https://alpha.example.org/')
    expect(s.prompt).toContain('Start with: /, /docs')
  })

  it('drafts a tools.yaml from the forms when nothing was found', () => {
    const { listings } = loadDirectory(FIXTURES)
    const report = { ...listings[0].report, tools: [], counts: { ...listings[0].report.counts, tools: 0 } }
    const s = sightkickStarter(report)
    expect(s.kind).toBe('author')
    expect(s.lang).toBe('yaml')
    expect(s.code).toContain('- name: search')
    expect(s.code).toContain('fill: { query: Form1Field_q, value: "{{q}}" }')
    expect(s.code).toContain('name: alpha-example-org')
  })
})
