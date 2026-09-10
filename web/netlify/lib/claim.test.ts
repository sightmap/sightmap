// The claim check, with a fake fetch standing in for the submitted host.
//
// Every case here is a way the check can be wrong in the submitter's favour if
// it is written loosely: a redirect onto a host they do not control, a file
// they can make arbitrarily large, a request that never returns.

import { describe, expect, it, vi } from 'vitest'
import {
  CLAIM_TOKEN_RE,
  claimLine,
  claimUrl,
  hasClaim,
  verifyClaim,
} from './claim.ts'

const TOKEN = 'a'.repeat(31) + '9'
const OTHER = 'b'.repeat(31) + '1'
const FILE = `https://example.org/webmcp.txt\n# sightmap-claim: ${TOKEN}\nsearch — Search the catalogue\n`

/** A fetch that answers one URL at a time from a script of responses. */
function fakeFetch(responses: Record<string, Response | (() => Response)>) {
  return vi.fn(async (input: RequestInfo | URL) => {
    const key = String(input)
    const found = responses[key]
    if (!found) throw new TypeError(`fetch failed: no route for ${key}`)
    return typeof found === 'function' ? found() : found
  }) as unknown as typeof fetch
}

const redirect = (to: string, status = 301): Response =>
  new Response(null, { status, headers: { location: to } })

describe('claimUrl', () => {
  it('asks the submitted host for its own webmcp.txt', () => {
    expect(claimUrl('https://example.org/pricing?ref=x')).toBe('https://example.org/webmcp.txt')
    expect(claimUrl('example.org')).toBe('https://example.org/webmcp.txt')
    // An http:// submission never reaches here, but the file is still https.
    expect(claimUrl('http://example.org/')).toBe('https://example.org/webmcp.txt')
  })
})

describe('claimLine', () => {
  it('matches the documented comment line anywhere in the file', () => {
    expect(claimLine(TOKEN).test(`#sightmap-claim:${TOKEN}`)).toBe(true)
    expect(claimLine(TOKEN).test(`#   sightmap-claim:   ${TOKEN}   `)).toBe(true)
    expect(claimLine(TOKEN).test(`first line\n# sightmap-claim: ${TOKEN}\nmore`)).toBe(true)
  })

  it('does not match a line that only contains the token', () => {
    expect(claimLine(TOKEN).test(`sightmap-claim: ${TOKEN}`)).toBe(false)
    expect(claimLine(TOKEN).test(`# claim ${TOKEN}`)).toBe(false)
  })

  it('does not match a different token', () => {
    expect(hasClaim(FILE, OTHER)).toBe(false)
    expect(hasClaim(FILE, TOKEN)).toBe(true)
  })

  it('rejects anything that is not 32 lowercase hex characters', () => {
    expect(CLAIM_TOKEN_RE.test(TOKEN)).toBe(true)
    expect(CLAIM_TOKEN_RE.test(TOKEN.toUpperCase())).toBe(false)
    expect(CLAIM_TOKEN_RE.test(TOKEN.slice(1))).toBe(false)
    expect(CLAIM_TOKEN_RE.test(`${TOKEN}0`)).toBe(false)
  })
})

describe('verifyClaim', () => {
  it('accepts a file that carries the line', async () => {
    const fetchImpl = fakeFetch({ 'https://example.org/webmcp.txt': new Response(FILE, { status: 200 }) })
    await expect(verifyClaim('https://example.org/pricing', TOKEN, fetchImpl)).resolves.toEqual({ ok: true })
  })

  it('reports a mismatch when the file is there but the line is not', async () => {
    const fetchImpl = fakeFetch({
      'https://example.org/webmcp.txt': new Response('https://example.org/\nsearch — Search\n', { status: 200 }),
    })
    const result = await verifyClaim('https://example.org/', TOKEN, fetchImpl)
    expect(result).toMatchObject({ ok: false, code: 'claim-mismatch' })
  })

  it('follows a same-site www redirect', async () => {
    const fetchImpl = fakeFetch({
      'https://example.org/webmcp.txt': redirect('https://www.example.org/webmcp.txt'),
      'https://www.example.org/webmcp.txt': new Response(FILE, { status: 200 }),
    })
    await expect(verifyClaim('https://example.org/', TOKEN, fetchImpl)).resolves.toEqual({ ok: true })
  })

  it('refuses a redirect that leaves the site', async () => {
    // The whole point of the check: a token on someone else's server proves
    // nothing about this host.
    const fetchImpl = fakeFetch({
      'https://example.org/webmcp.txt': redirect('https://pages.example.net/webmcp.txt'),
      'https://pages.example.net/webmcp.txt': new Response(FILE, { status: 200 }),
    })
    const result = await verifyClaim('https://example.org/', TOKEN, fetchImpl)
    expect(result).toEqual({ ok: false, code: 'claim-unreachable', reason: 'redirected off-site' })
  })

  it('stops after three redirects', async () => {
    const hop = (n: number) => `https://example.org/webmcp.txt?${n}`
    const fetchImpl = fakeFetch({
      'https://example.org/webmcp.txt': redirect(hop(1)),
      [hop(1)]: redirect(hop(2)),
      [hop(2)]: redirect(hop(3)),
      [hop(3)]: redirect(hop(4)),
      [hop(4)]: new Response(FILE, { status: 200 }),
    })
    const result = await verifyClaim('https://example.org/', TOKEN, fetchImpl)
    expect(result).toMatchObject({ ok: false, code: 'claim-unreachable' })
    expect((result as { reason: string }).reason).toContain('redirects')
  })

  it('refuses a file bigger than the cap, without reading all of it', async () => {
    const oversized = `${'x'.repeat(200)}\n`.repeat(10)
    const fetchImpl = fakeFetch({
      'https://example.org/webmcp.txt': new Response(oversized, { status: 200 }),
    })
    const result = await verifyClaim('https://example.org/', TOKEN, fetchImpl, { maxBytes: 128 })
    expect(result).toMatchObject({ ok: false, code: 'claim-unreachable' })
    expect((result as { reason: string }).reason).toContain('larger than')
  })

  it('refuses a declared Content-Length over the cap', async () => {
    const fetchImpl = fakeFetch({
      'https://example.org/webmcp.txt': new Response(FILE, {
        status: 200,
        headers: { 'content-length': String(1024 * 1024) },
      }),
    })
    const result = await verifyClaim('https://example.org/', TOKEN, fetchImpl, { maxBytes: 128 })
    expect(result).toMatchObject({ ok: false, code: 'claim-unreachable' })
  })

  it('gives up on a host that never answers', async () => {
    const hang = vi.fn(
      (_input: RequestInfo | URL, init?: RequestInit) =>
        new Promise<Response>((_resolve, reject) => {
          init?.signal?.addEventListener('abort', () => reject(new Error('aborted')))
        })
    ) as unknown as typeof fetch
    const result = await verifyClaim('https://example.org/', TOKEN, hang, { timeoutMs: 5 })
    expect(result).toMatchObject({ ok: false, code: 'claim-unreachable' })
    expect((result as { reason: string }).reason).toContain('timed out')
  })

  it('treats a missing file as unreachable, not as a mismatch', async () => {
    const fetchImpl = fakeFetch({
      'https://example.org/webmcp.txt': new Response('not found', { status: 404 }),
    })
    const result = await verifyClaim('https://example.org/', TOKEN, fetchImpl)
    expect(result).toEqual({ ok: false, code: 'claim-unreachable', reason: 'HTTP 404' })
  })

  it('reports a transport failure without throwing', async () => {
    const fetchImpl = fakeFetch({})
    const result = await verifyClaim('https://example.org/', TOKEN, fetchImpl)
    expect(result).toMatchObject({ ok: false, code: 'claim-unreachable' })
  })

  it('never puts the token in the reason it returns', async () => {
    const fetchImpl = fakeFetch({
      'https://example.org/webmcp.txt': new Response('nothing here', { status: 200 }),
    })
    const result = await verifyClaim('https://example.org/', TOKEN, fetchImpl)
    expect(JSON.stringify(result)).not.toContain(TOKEN)
  })
})
