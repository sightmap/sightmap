// Content negotiation + JSON API errors for sightmap.org.
//
// The edge function is a thin wrapper around handleNegotiate so the
// Accept / 404 / API behavior can be tested in Node without Deno.

import {
  isPassthroughPath,
  markdownTwinPath,
  mergeVary,
  negotiate,
  normalizePathname,
} from './accept.ts'
import {
  methodNotAllowedError,
  notAcceptableBody,
  notFoundError,
  type ApiErrorBody,
} from './errors.ts'

export const MARKDOWN_TYPE = 'text/markdown; charset=utf-8'
export const JSON_TYPE = 'application/json; charset=utf-8'
export const PLAIN_TYPE = 'text/plain; charset=utf-8'

const PAGE_TYPES = ['text/html', 'text/markdown'] as const
const ERROR_TYPES = ['text/html', 'text/markdown', 'application/json'] as const
const VARY_ACCEPT = ['Accept', 'Accept-Encoding'] as const

export interface NegotiateDeps {
  next: () => Promise<Response>
  fetch: (input: URL) => Promise<Response>
}

function withVary(res: Response): Response {
  const headers = new Headers(res.headers)
  headers.set('Vary', mergeVary(headers.get('Vary'), VARY_ACCEPT))
  return new Response(res.body, { status: res.status, statusText: res.statusText, headers })
}

function jsonResponse(body: unknown, status: number, extra?: HeadersInit): Response {
  const headers = new Headers(extra)
  headers.set('Content-Type', JSON_TYPE)
  headers.set('Vary', mergeVary(headers.get('Vary'), VARY_ACCEPT))
  headers.set('Cache-Control', status >= 400 ? 'no-store' : 'public, max-age=300')
  return new Response(JSON.stringify(body), { status, headers })
}

function markdownResponse(body: string, status: number): Response {
  return new Response(body, {
    status,
    headers: {
      'Content-Type': MARKDOWN_TYPE,
      Vary: mergeVary(null, VARY_ACCEPT),
      'Cache-Control': status >= 400 ? 'no-store' : 'public, max-age=300',
    },
  })
}

function notAcceptable(available: readonly string[], requested: string): Response {
  return new Response(notAcceptableBody(available, requested), {
    status: 406,
    headers: {
      'Content-Type': PLAIN_TYPE,
      Vary: mergeVary(null, VARY_ACCEPT),
      'Cache-Control': 'no-store',
    },
  })
}

function errorJson(body: ApiErrorBody): Response {
  return jsonResponse(body, body.error.status)
}

async function fetchSameOrigin(deps: NegotiateDeps, request: Request, path: string): Promise<Response> {
  return deps.fetch(new URL(path, request.url))
}

async function handleApi(request: Request, path: string, deps: NegotiateDeps): Promise<Response> {
  if (request.method !== 'GET' && request.method !== 'HEAD') {
    return errorJson(methodNotAllowedError(request.method, path))
  }

  if (path === '/api/atlas') {
    const upstream = await fetchSameOrigin(deps, request, '/api/atlas.json')
    if (!upstream.ok) return errorJson(notFoundError(path))
    const body = await upstream.text()
    return jsonResponse(JSON.parse(body), 200)
  }

  const entry = path.match(/^\/api\/atlas\/([^/]+)$/)
  if (entry) {
    const slug = entry[1]
    const upstream = await fetchSameOrigin(deps, request, `/api/atlas/${slug}.json`)
    if (!upstream.ok) {
      return errorJson(
        notFoundError(path)
      )
    }
    const body = await upstream.text()
    return jsonResponse(JSON.parse(body), 200)
  }

  return errorJson(notFoundError(path))
}

async function markdownFor(request: Request, path: string, status: number, deps: NegotiateDeps): Promise<Response> {
  const twin = status === 404 ? '/404.md' : markdownTwinPath(path)
  const upstream = await fetchSameOrigin(deps, request, twin)
  if (upstream.ok) {
    return markdownResponse(await upstream.text(), status)
  }
  if (status !== 404) {
    const fallback = await fetchSameOrigin(deps, request, '/404.md')
    if (fallback.ok) return markdownResponse(await fallback.text(), 404)
  }
  return markdownResponse(
    '# Not found\n\nThis path is not on sightmap.org.\n\nSee https://sightmap.org/llms.txt\n',
    404
  )
}

export async function handleNegotiate(
  request: Request,
  deps: NegotiateDeps
): Promise<Response | undefined> {
  const url = new URL(request.url)
  const path = normalizePathname(url.pathname)

  if (isPassthroughPath(path)) return undefined

  if (path === '/api' || path.startsWith('/api/')) {
    return handleApi(request, path, deps)
  }

  const accept = request.headers.get('Accept')
  const origin = await deps.next()

  if (origin.status === 404) {
    const chosen = negotiate(accept, ERROR_TYPES, 'text/html')
    if (chosen === null) return notAcceptable(ERROR_TYPES, accept ?? '')
    if (chosen === 'text/markdown') return markdownFor(request, path, 404, deps)
    if (chosen === 'application/json') return errorJson(notFoundError(path))
    return withVary(origin)
  }

  const chosen = negotiate(accept, PAGE_TYPES, 'text/html')
  if (chosen === null) {
    // Client refused HTML and Markdown. application/json is only a 404
    // representation, so an existing page cannot satisfy it.
    const asError = negotiate(accept, ERROR_TYPES, 'text/html')
    if (asError === 'application/json') return notAcceptable(PAGE_TYPES, accept ?? '')
    return notAcceptable(PAGE_TYPES, accept ?? '')
  }
  if (chosen === 'text/markdown') return markdownFor(request, path, origin.status, deps)
  return withVary(origin)
}
