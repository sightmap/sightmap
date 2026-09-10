// GET /try/<host> — the unlisted launch card for a claimed, scanned site.
//
// A thin wrapper, like atlas-submit.mts: the whole decision is
// decideTryCard() and the whole page is renderTryCard(), both pure and both
// tested in ../lib/try-card.test.ts. What is left here is the I/O — one
// same-origin fetch and two Blobs reads — plus the status codes.
//
// The submitted site is never contacted. Not fetched, proxied, framed or
// screenshotted: a card is built from the stored record alone, so a visit to
// this page cannot be turned into traffic, a probe, or a request carrying a
// visitor's address to a stranger's server.
//
// Card pages are unlisted by design (`noindex`, `Cache-Control: no-store`) and
// deliberately not part of the prerendered site: a listing means a maintainer
// looked, and this page means nobody has.

import { getStore } from '@netlify/blobs'
import type { Context } from '@netlify/functions'
import { canonicalHost } from '../../scripts/lib/directory.ts'
import { preflightUrl } from '../../scripts/lib/preflight.ts'
import { decideTryCard, renderGone, renderNotFound, renderTryCard } from '../lib/try-card.ts'
import { QUARANTINE_STORE, TRY_STORE, type TryRecord } from '../lib/try-record.ts'

export const config = {
  // Netlify evaluates functions before redirects, so this path wins over the
  // /* → /404.html catch-all in netlify.toml without a rule of its own — the
  // same chain atlas-submit.mts documents. Nothing is prerendered under /try/,
  // so no static file shadows it either.
  path: ['/try/:host'],
}

const HTML_TYPE = 'text/html; charset=utf-8'

/** A listing slug is a path segment we are about to put in a Location header. */
const SAFE_SLUG = /^[a-z0-9][a-z0-9-]*$/

function html(body: string, status: number): Response {
  return new Response(body, {
    status,
    headers: {
      'Content-Type': HTML_TYPE,
      'Cache-Control': 'no-store',
      // The meta tag is the one the contract asks for; the header covers the
      // crawlers and unfurlers that never parse the document.
      'X-Robots-Tag': 'noindex, nofollow',
    },
  })
}

/**
 * The slug of the Atlas listing for this host, or null if it has none.
 *
 * Read from the deploy's own /atlas/hosts/<host>.json, which build-atlas.ts
 * writes for every listed host — a 404 there is the answer "not admitted", and
 * so is an unreachable origin: the card below claims less than a listing does,
 * so degrading towards it is safe in a way the reverse would not be.
 */
async function listedSlug(origin: string, host: string): Promise<string | null> {
  try {
    const res = await fetch(`${origin}/atlas/hosts/${encodeURIComponent(host)}.json`)
    if (!res.ok) return null
    const doc = (await res.json()) as { slug?: unknown }
    const slug = typeof doc.slug === 'string' ? doc.slug : ''
    return SAFE_SLUG.test(slug) ? slug : null
  } catch (err) {
    console.warn(`[atlas-try] listing lookup failed for ${host}: ${message(err)}`)
    return null
  }
}

function message(err: unknown): string {
  return err instanceof Error ? err.message : String(err)
}

/**
 * The quarantine flag and the record, in one read pair.
 *
 * Blobs is best-effort here as it is in atlas-submit.mts: a failure is one
 * warning and a null, which decideTryCard reads as "no card" — a 404. The
 * alternative, a 500 with the store's error in it, would tell a stranger about
 * our storage and still not show them a card.
 */
async function loadRecord(
  host: string
): Promise<{ quarantined: boolean; record: TryRecord | null }> {
  try {
    const [quarantine, record] = await Promise.all([
      getStore(QUARANTINE_STORE).get(host, { type: 'json' }),
      getStore(TRY_STORE).get(host, { type: 'json' }),
    ])
    return { quarantined: quarantine != null, record: (record as TryRecord | null) ?? null }
  } catch (err) {
    console.warn(`[atlas-try] blobs lookup failed for ${host}: ${message(err)}`)
    return { quarantined: false, record: null }
  }
}

export default async (req: Request, context: Context): Promise<Response> => {
  const url = new URL(req.url)

  if (req.method !== 'GET' && req.method !== 'HEAD') {
    return new Response(null, {
      status: 405,
      headers: { Allow: 'GET, HEAD', 'Cache-Control': 'no-store' },
    })
  }

  // The path parameter is percent-decoded by the runtime, but a hand-written
  // URL can still carry an escape the runtime left alone.
  let raw = context.params?.host ?? url.pathname.split('/').pop() ?? ''
  try {
    raw = decodeURIComponent(raw)
  } catch {
    return html(renderNotFound(), 404)
  }

  // Records are keyed by the canonical host — the same string the Atlas keys
  // /atlas/hosts/<host>.json by — so /try/www.example.com and
  // /try/example.com are one card, not two.
  const host = canonicalHost(raw)
  const preflight = preflightUrl(`https://${host}`)

  // Each lookup is skipped once an earlier check has already decided; see the
  // note on TryDecisionInputs for why the skipped ones are safe to pass empty.
  const origin = process.env.URL?.replace(/\/+$/, '') || url.origin
  const slug = preflight.ok ? await listedSlug(origin, host) : null
  const stored = preflight.ok && !slug ? await loadRecord(host) : { quarantined: false, record: null }

  const decision = decideTryCard({
    preflightOk: preflight.ok,
    listedSlug: slug,
    quarantined: stored.quarantined,
    record: stored.record,
  })

  switch (decision.kind) {
    case 'listed':
      // Admitted since the card was made: the listing is the better page, and
      // it is the one that survives being indexed.
      return new Response(null, {
        status: 302,
        headers: { Location: `/atlas/${decision.slug}`, 'Cache-Control': 'no-store' },
      })
    case 'gone':
      return html(renderGone(), 410)
    case 'card':
      // Rendered against the canonical site, not `origin`: a share or an
      // unfurl of a card served from a deploy preview should still point at
      // the address that will still be there tomorrow.
      return html(renderTryCard(decision.record), 200)
    default:
      return html(renderNotFound(), 404)
  }
}
