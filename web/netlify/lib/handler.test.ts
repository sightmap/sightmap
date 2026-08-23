import { describe, expect, it } from 'vitest'
import { handleNegotiate, JSON_TYPE, MARKDOWN_TYPE } from './handler'

function request(path: string, headers: Record<string, string> = {}, method = 'GET') {
  return new Request(`https://sightmap.org${path}`, { method, headers })
}

function deps(opts: {
  origin?: Response
  files?: Record<string, Response>
}): { next: () => Promise<Response>; fetch: (input: URL) => Promise<Response> } {
  return {
    next: async () =>
      opts.origin ??
      new Response('<html>ok</html>', {
        status: 200,
        headers: { 'content-type': 'text/html; charset=utf-8', vary: 'accept-encoding' },
      }),
    fetch: async (input) => {
      const path = new URL(input).pathname
      const found = opts.files?.[path]
      if (found) return found.clone()
      return new Response('missing', { status: 404 })
    },
  }
}

describe('handleNegotiate', () => {
  it('passes through extension-bearing paths so twin fetches do not loop', async () => {
    const res = await handleNegotiate(request('/openapi.json'), deps({}))
    expect(res).toBeUndefined()
  })

  it('returns JSON 404 for an unknown /api path', async () => {
    const res = await handleNegotiate(request('/api/nope'), deps({}))
    expect(res).toBeDefined()
    expect(res!.status).toBe(404)
    expect(res!.headers.get('content-type')).toBe(JSON_TYPE)
    const body = (await res!.json()) as { error: { code: string; hint: string } }
    expect(body.error.code).toBe('not_found')
    expect(body.error.hint).toContain('llms.txt')
    expect(res!.headers.get('vary')?.toLowerCase()).toContain('accept')
  })

  it('rejects non-GET methods on the API with JSON 405', async () => {
    const res = await handleNegotiate(request('/api/atlas', {}, 'POST'), deps({}))
    expect(res!.status).toBe(405)
    const body = (await res!.json()) as { error: { code: string } }
    expect(body.error.code).toBe('method_not_allowed')
  })

  it('proxies GET /api/atlas to the generated catalog', async () => {
    const res = await handleNegotiate(
      request('/api/atlas'),
      deps({
        files: {
          '/api/atlas.json': new Response(JSON.stringify({ schema_version: 1, entries: [] }), {
            status: 200,
          }),
        },
      })
    )
    expect(res!.status).toBe(200)
    expect(res!.headers.get('content-type')).toBe(JSON_TYPE)
    expect(await res!.json()).toEqual({ schema_version: 1, entries: [] })
  })

  it('returns JSON 404 for a missing atlas slug', async () => {
    const res = await handleNegotiate(request('/api/atlas/missing'), deps({ files: {} }))
    expect(res!.status).toBe(404)
    const body = (await res!.json()) as { error: { code: string } }
    expect(body.error.code).toBe('not_found')
  })

  it('serves the markdown 404 body for Accept: text/markdown on a missing page', async () => {
    const res = await handleNegotiate(
      request('/no-such-page', { Accept: 'text/markdown' }),
      deps({
        origin: new Response('<html>404</html>', { status: 404, headers: { 'content-type': 'text/html' } }),
        files: {
          '/404.md': new Response('# Not found\n\nSee https://sightmap.org/llms.txt\n', {
            status: 200,
          }),
        },
      })
    )
    expect(res!.status).toBe(404)
    expect(res!.headers.get('content-type')).toBe(MARKDOWN_TYPE)
    expect(res!.headers.get('vary')?.toLowerCase()).toContain('accept')
    expect(await res!.text()).toContain('llms.txt')
  })

  it('serves a JSON 404 when Accept prefers application/json on a missing page', async () => {
    const res = await handleNegotiate(
      request('/no-such-page', { Accept: 'application/json' }),
      deps({
        origin: new Response('<html>404</html>', { status: 404 }),
      })
    )
    expect(res!.status).toBe(404)
    expect(res!.headers.get('content-type')).toBe(JSON_TYPE)
    const body = (await res!.json()) as { error: { code: string } }
    expect(body.error.code).toBe('not_found')
  })

  it('serves the markdown twin of an existing page and sets Vary: Accept', async () => {
    const res = await handleNegotiate(
      request('/blog', { Accept: 'text/markdown' }),
      deps({
        files: {
          '/blog.md': new Response('# Blog — Sightmap\n', { status: 200 }),
        },
      })
    )
    expect(res!.status).toBe(200)
    expect(res!.headers.get('content-type')).toBe(MARKDOWN_TYPE)
    expect(res!.headers.get('vary')).toMatch(/Accept/)
    expect(await res!.text()).toContain('# Blog — Sightmap')
  })

  it('adds Accept to Vary on the HTML representation', async () => {
    const res = await handleNegotiate(
      request('/blog', { Accept: 'text/html' }),
      deps({
        origin: new Response('<html>blog</html>', {
          status: 200,
          headers: { 'content-type': 'text/html', vary: 'accept-encoding' },
        }),
      })
    )
    expect(res!.status).toBe(200)
    expect(await res!.text()).toBe('<html>blog</html>')
    const vary = res!.headers.get('vary') ?? ''
    expect(vary.toLowerCase()).toContain('accept')
    expect(vary.toLowerCase()).toContain('accept-encoding')
  })

  it('returns 406 when Accept cannot be satisfied', async () => {
    const res = await handleNegotiate(
      request('/blog', { Accept: 'application/pdf' }),
      deps({})
    )
    expect(res!.status).toBe(406)
    expect(res!.headers.get('vary')?.toLowerCase()).toContain('accept')
    expect(await res!.text()).toContain('text/markdown')
  })
})
