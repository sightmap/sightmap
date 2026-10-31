// Function-level tests for the launch card endpoint.
//
// The decision and the HTML are pure and tested in ../lib/try-card.test.ts.
// What is left here is the wiring: which store and which same-origin URL each
// branch reads, the status codes, and the two invariants that only show up at
// this level — a Blobs failure degrades to a 404 rather than a 500, and the
// submitted site is never contacted.
//
// Two fakes stand in for the runtime: an in-memory Blobs read path keyed by
// store name, and a stubbed global fetch for the listing lookup.

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import handler from './atlas-try.mts'

/** `stores.get('atlas-try')` is one store's key → value map. */
const stores = new Map<string, Map<string, unknown>>()
/** Set to make the next store read throw, to exercise the degraded path. */
let storeFails = false

vi.mock('@netlify/blobs', () => ({
  getStore: (name: string) => ({
    get: async (key: string) => {
      if (storeFails) throw new Error('store unavailable')
      return stores.get(name)?.get(key) ?? null
    },
  }),
}))

/** The host document the deploy would serve for a host already in the Atlas. */
let listing: Record<string, unknown> | null = null
let fetched: string[] = []

beforeEach(() => {
  stores.clear()
  storeFails = false
  listing = null
  fetched = []
  delete process.env.URL
  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: string | URL) => {
      fetched.push(String(input))
      return listing
        ? new Response(JSON.stringify(listing), { status: 200 })
        : new Response('not found', { status: 404 })
    })
  )
})

afterEach(() => {
  vi.unstubAllGlobals()
  delete process.env.URL
})

function put(store: string, key: string, value: unknown): void {
  const s = stores.get(store) ?? new Map<string, unknown>()
  s.set(key, value)
  stores.set(store, s)
}

/** Netlify hands the path parameter to the handler on the context. */
const context = (host: string) => ({ params: { host } }) as never
const request = (host: string, init?: RequestInit) =>
  new Request(`https://sightmap.org/try/${host}`, init)

const RECORD = {
  v: 1,
  host: 'example.dev',
  url: 'https://example.dev/',
  submissionId: 'sub_1',
  claimedAt: '2026-09-01T00:00:00.000Z',
  expiresAt: '2099-01-01T00:00:00.000Z',
}

describe('GET /try/<host>', () => {
  it('404s a host that fails the shape rules, without any lookup', async () => {
    const res = await handler(request('localhost'), context('localhost'))
    expect(res.status).toBe(404)
    expect(res.headers.get('Cache-Control')).toBe('no-store')
    expect(fetched).toEqual([])
  })

  it('redirects a host the Atlas has admitted to its listing', async () => {
    listing = { slug: 'example-dev' }
    put('atlas-try', 'example.dev', RECORD)
    const res = await handler(request('example.dev'), context('example.dev'))
    expect(res.status).toBe(302)
    expect(res.headers.get('Location')).toBe('/atlas/example-dev')
  })

  it('ignores a slug that could not be a path segment', async () => {
    listing = { slug: '../../evil' }
    put('atlas-try', 'example.dev', RECORD)
    const res = await handler(request('example.dev'), context('example.dev'))
    expect(res.status).toBe(200)
  })

  it('reads the listing from the deploy URL when the environment names one', async () => {
    process.env.URL = 'https://deploy-preview-7--sightmap.netlify.app/'
    await handler(request('example.dev'), context('example.dev'))
    expect(fetched).toEqual([
      'https://deploy-preview-7--sightmap.netlify.app/atlas/hosts/example.dev.json',
    ])
  })

  it('falls back to the site URL, never to the request origin', async () => {
    const req = new Request('https://attacker.example/try/example.dev')
    await handler(req, context('example.dev'))
    expect(fetched).toEqual(['https://sightmap.org/atlas/hosts/example.dev.json'])
  })

  it('404s a segment that parses as more than a hostname, without any lookup', async () => {
    const res = await handler(request('example.dev:8443'), context('example.dev:8443'))
    expect(res.status).toBe(404)
    expect(fetched).toEqual([])
  })

  it('answers 410 for a quarantined host', async () => {
    put('atlas-quarantine', 'example.dev', { at: '2026-09-10T00:00:00.000Z' })
    put('atlas-try', 'example.dev', RECORD)
    const res = await handler(request('example.dev'), context('example.dev'))
    expect(res.status).toBe(410)
    expect(await res.text()).toContain('This card was removed.')
  })

  it('renders the card for a live record, keyed by the canonical host', async () => {
    put('atlas-try', 'example.dev', RECORD)
    const res = await handler(request('www.example.dev'), context('www.example.dev'))
    expect(res.status).toBe(200)
    expect(res.headers.get('Content-Type')).toBe('text/html; charset=utf-8')
    expect(res.headers.get('Cache-Control')).toBe('no-store')
    expect(res.headers.get('X-Robots-Tag')).toBe('noindex, nofollow')
    expect(await res.text()).toContain('<h1>https://example.dev</h1>')
  })

  it('never contacts the submitted site', async () => {
    put('atlas-try', 'example.dev', RECORD)
    await handler(request('example.dev'), context('example.dev'))
    expect(fetched).toEqual(['https://sightmap.org/atlas/hosts/example.dev.json'])
  })

  it('404s an expired record', async () => {
    put('atlas-try', 'example.dev', { ...RECORD, expiresAt: '2020-01-01T00:00:00.000Z' })
    const res = await handler(request('example.dev'), context('example.dev'))
    expect(res.status).toBe(404)
  })

  it('degrades to 404 when the store cannot be read', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    storeFails = true
    const res = await handler(request('example.dev'), context('example.dev'))
    expect(res.status).toBe(404)
    expect(warn).toHaveBeenCalledTimes(1)
    // Never the store's own error text, and never a 500.
    expect(await res.text()).not.toContain('store unavailable')
    warn.mockRestore()
  })

  it('refuses anything but a read', async () => {
    const res = await handler(request('example.dev', { method: 'POST' }), context('example.dev'))
    expect(res.status).toBe(405)
    expect(res.headers.get('Allow')).toBe('GET, HEAD')
  })
})
