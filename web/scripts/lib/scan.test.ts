import { execFileSync } from 'node:child_process'
import fs from 'node:fs'
import http from 'node:http'
import path from 'node:path'
import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import { pickLinks, scanSite } from './scan'

const FIXTURE = path.resolve(__dirname, '__fixtures__/webmcp-site')

// The scan needs the sightmap CLI and a Chrome build. SIGHTMAP_BIN points at
// the CLI (default: `sightmap` on PATH) and ATLAS_CHROME_PATH at a Chrome
// binary (default: the CLI's own `browser install`). Without a working CLI the
// browser-driven cases are skipped rather than failed, so `pnpm test` stays
// green on a machine that has never installed it.
function haveSightmap(): boolean {
  try {
    execFileSync(process.env.SIGHTMAP_BIN || 'sightmap', ['version'], { stdio: 'pipe', timeout: 10_000 })
    return true
  } catch {
    return false
  }
}

const HAVE_CLI = haveSightmap()

describe('pickLinks', () => {
  it('keeps same-origin, unseen, non-destructive links in order, preferred paths first', () => {
    const picked = pickLinks(
      'https://a.example',
      '/',
      ['/docs', 'https://a.example/pricing#x', '/logout', 'https://b.example/', '/docs', 'mailto:x@y', '/cart'],
      3,
      ['/about']
    )
    expect(picked).toEqual(['https://a.example/about', 'https://a.example/docs', 'https://a.example/pricing'])
  })
})

describe.skipIf(!HAVE_CLI)('scanSite against the fixture site', () => {
  let server: http.Server
  let base = ''

  beforeAll(async () => {
    server = http.createServer((req, res) => {
      const url = new URL(req.url ?? '/', 'http://127.0.0.1')
      const file = url.pathname === '/' ? 'index.html' : url.pathname.slice(1)
      const full = path.join(FIXTURE, file)
      if (!full.startsWith(FIXTURE) || !fs.existsSync(full)) {
        res.writeHead(404).end('nope')
        return
      }
      res.writeHead(200, { 'content-type': 'text/html; charset=utf-8' }).end(fs.readFileSync(full))
    })
    await new Promise<void>((r) => server.listen(0, '127.0.0.1', r))
    const addr = server.address()
    if (addr && typeof addr === 'object') base = `http://127.0.0.1:${addr.port}`
  })

  afterAll(async () => {
    await new Promise<void>((r) => server.close(() => r()))
  })

  it('records imperative and declarative tools across pages without executing any', async () => {
    const report = await scanSite({ url: `${base}/`, allowLocal: true, maxPages: 3 })

    expect(report.status).toBe('needs-review') // the injected "Legacy Helper" description trips the wording check
    expect(report.surface).toBe('polyfilled')
    expect(report.pages.map((p) => p.path)).toEqual(['/', '/docs.html'])
    expect(report.pages[0].tools.sort()).toEqual(['Legacy Helper', 'add_to_cart', 'place_order', 'search_products', 'subscribe_newsletter'].sort())
    expect(report.pages[1].tools.sort()).toEqual(['get_doc', 'search_products'])

    const byName = Object.fromEntries(report.tools.map((t) => [t.name, t]))
    expect(byName.search_products).toMatchObject({ risk: 'read', impl: 'imperative', api: 'navigator', pages: ['/', '/docs.html'] })
    expect(byName.add_to_cart.risk).toBe('action')
    expect(byName.place_order.risk).toBe('sensitive')
    expect(byName.subscribe_newsletter).toMatchObject({ impl: 'declarative', api: 'dom', risk: 'sensitive' })
    expect((byName.subscribe_newsletter.inputSchema as { required: string[] }).required).toEqual(['email'])
    expect(byName['Legacy Helper']).toMatchObject({ api: 'document' })
    expect(byName['Legacy Helper'].warnings).toContain('name is not snake_case')
    expect(byName['Legacy Helper'].warnings).toContain('no input schema')
    expect(byName['Legacy Helper'].warnings.some((w) => w.startsWith('metadata matches'))).toBe(true)

    expect(report.counts).toMatchObject({ tools: 6, pages: 2, read: 2, sensitive: 2, declarative: 1 })
    expect(report.checks.find((c) => c.id === 'route-scoped')?.ok).toBe(true)
    expect(report.hints.forms).toEqual([{ page: '/', action: '/search', method: 'get', fields: ['q'] }])
    expect(report.hints.title).toBe('Fixture Shop')
  }, 60_000)

  it('reports api-absent for a page that registers nothing', async () => {
    const report = await scanSite({ url: `${base}/docs.html`, allowLocal: true, maxPages: 1 })
    // docs.html does register via provideContext, so use a 404 page for absence.
    expect(report.status).toBe('tools-found')
    const missing = await scanSite({ url: `${base}/nothing.html`, allowLocal: true, maxPages: 1 })
    expect(missing.status).toBe('api-absent')
    expect(missing.pages[0].status).toBe(404)
  }, 60_000)

  it('records a page that brings its own polyfill, replaces the surface, and registers late', async () => {
    const report = await scanSite({ url: `${base}/polyfilled.html`, allowLocal: true, maxPages: 1 })

    expect(report.status).toBe('tools-found')
    expect(report.surface).toBe('polyfilled')
    expect(report.pages[0].error).toBeUndefined()
    expect(report.pages[0].tools.sort()).toEqual(
      ['check_stock', 'find_warranty', 'list_stores', 'quote_shipping', 'track_package'].sort()
    )

    const byName = Object.fromEntries(report.tools.map((t) => [t.name, t]))
    // Registered through `document.modelContext ?? navigator.modelContext`
    // while the page's own polyfill stood down: recorded on our surface.
    expect(byName.track_package).toMatchObject({ api: 'document', impl: 'imperative' })
    // Assigned over the recorder — the accessor's setter adopted it.
    expect(byName.list_stores).toMatchObject({ api: 'navigator', impl: 'imperative' })
    // Registered on a surface that was defineProperty'd over ours, so it is
    // only visible by enumerating the replacement at collect time.
    expect(byName.quote_shipping).toMatchObject({ api: 'navigator', impl: 'imperative' })
    expect((byName.quote_shipping.inputSchema as { required: string[] }).required).toEqual(['destination'])
    // Registered 800ms in — past the old fixed settle window.
    expect(byName.check_stock).toMatchObject({ api: 'document', impl: 'imperative' })
    // Late *and* on the replaced surface.
    expect(byName.find_warranty).toMatchObject({ api: 'navigator', impl: 'imperative' })

    expect(report.counts).toMatchObject({ tools: 5, pages: 1, declarative: 0 })
    // A script that never loaded is named, not silently missing.
    expect(report.notes.some((n) => n.includes('script failed to load') && n.includes('missing-polyfill.js'))).toBe(true)
    expect(report.notes.some((n) => n.includes('replaced the navigator.modelContext surface'))).toBe(true)
  }, 60_000)

  it('refuses a loopback URL unless the caller opts in', async () => {
    await expect(scanSite({ url: `${base}/` })).rejects.toThrow(/preflight/)
  })
})
