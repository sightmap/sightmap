// netlify/edge-functions/negotiate.ts
//
// Accept-negotiates HTML pages as text/markdown, returns structured JSON
// errors for /api/* and for 404s that asked for JSON, and always sets
// Vary: Accept so a CDN cannot serve the HTML variant to an agent (or
// the markdown variant to a browser).
import type { Context } from '@netlify/edge-functions'
import { handleNegotiate } from '../lib/handler.ts'

export default async function handler(
  request: Request,
  context: Context
): Promise<Response | undefined> {
  return handleNegotiate(request, {
    next: () => context.next(),
    fetch: (input) => fetch(input),
  })
}
