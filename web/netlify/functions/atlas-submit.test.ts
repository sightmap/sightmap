// Function-level tests for the Atlas submission endpoint.
//
// Everything deterministic lives in ../lib/submit.ts and ../lib/runner.ts and is
// tested there. What is left here is the wiring the pure functions cannot see:
// which Blobs keys the handler touches, and the branch where the global daily
// runner ceiling is reached — the submission is still stored and still answered
// 202, but no runner is triggered.
//
// Two fakes stand in for the runtime: an in-memory Blobs store keyed by store
// name, and a stubbed global fetch (the runner trigger, and the claimed host's
// webmcp.txt). DNS is stubbed too so `resolvePublic` never leaves the machine.

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import handler from './atlas-submit.mts'
import { RUN_CLAIM_ATTEMPTS } from '../lib/runner.ts'
import { QUARANTINE_STORE, TRY_STORE, type TryRecord } from '../lib/try-record.ts'

/** In-memory Blobs: `stores.get('atlas-rate')` is one store's key → value map. */
const stores = new Map<string, Map<string, unknown>>()
/** ETags, per store, so the mock can honour `onlyIfMatch` / `onlyIfNew`. */
const etags = new Map<string, Map<string, string>>()
/** Store names whose next operation should throw, to exercise the fail-open paths. */
const failing = new Set<string>()
/**
 * `<store>/<key>` → how many upcoming writes should lose a race: a competing
 * writer lands between the read and the write that many times.
 */
const raceOnNextWrite = new Map<string, number>()

let etagSeq = 0

function store(name: string): Map<string, unknown> {
  let s = stores.get(name)
  if (!s) {
    s = new Map()
    stores.set(name, s)
  }
  return s
}

function etagsFor(name: string): Map<string, string> {
  let s = etags.get(name)
  if (!s) {
    s = new Map()
    etags.set(name, s)
  }
  return s
}

function write(name: string, key: string, value: unknown): string {
  const etag = `etag-${(etagSeq += 1)}`
  store(name).set(key, value)
  etagsFor(name).set(key, etag)
  return etag
}

vi.mock('@netlify/blobs', () => ({
  getStore: (name: string) => ({
    async get(key: string) {
      if (failing.has(name)) throw new Error(`blobs unavailable: ${name}`)
      return store(name).get(key) ?? null
    },
    async getWithMetadata(key: string) {
      if (failing.has(name)) throw new Error(`blobs unavailable: ${name}`)
      if (!store(name).has(key)) return null
      return { data: store(name).get(key), etag: etagsFor(name).get(key), metadata: {} }
    },
    async setJSON(key: string, value: unknown, options?: { onlyIfNew?: boolean; onlyIfMatch?: string }) {
      if (failing.has(name)) throw new Error(`blobs unavailable: ${name}`)

      // Simulate another invocation writing the same key first.
      const race = `${name}/${key}`
      const remaining = raceOnNextWrite.get(race) ?? 0
      if (remaining > 0) {
        raceOnNextWrite.set(race, remaining - 1)
        const current = store(name).get(key) as { count?: number } | undefined
        write(name, key, { count: (current?.count ?? 0) + 1 })
      }

      const exists = store(name).has(key)
      if (options?.onlyIfNew && exists) return { modified: false }
      if (options?.onlyIfMatch !== undefined && options.onlyIfMatch !== etagsFor(name).get(key)) {
        return { modified: false }
      }
      return { modified: true, etag: write(name, key, value) }
    },
  }),
}))

// preflight.ts resolves the submitted host before we accept it. One public
// address for everything; no network.
vi.mock('node:dns', async (importOriginal) => {
  const actual = (await importOriginal()) as { default: unknown }
  return {
    ...actual,
    default: { ...(actual.default as object), promises: { lookup: async () => [{ address: '93.184.216.34' }] } },
  }
})


const RATE_STORE = 'atlas-rate'
const SUBMISSIONS_STORE = 'atlas-submissions'
const RUNS_KEY = `runs/${new Date().toISOString().slice(0, 10)}`

const HOST = 'sightmap.org'
const TOKEN = '0123456789abcdef0123456789abcdef'
const CLAIM_FILE = `https://sightmap.org/\n# sightmap-claim: ${TOKEN}\nsearch — Search the Atlas\n`
/** What the claimed host serves at /webmcp.txt for the test at hand. */
let webmcp: () => Response

function post(body: Record<string, unknown> = {}): Request {
  return new Request('https://sightmap.org/api/atlas/submit', {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ url: 'https://sightmap.org/atlas', email: 'chip@sightmap.org', ...body }),
  })
}

function formPost(fields: Record<string, string> = {}): Request {
  return new Request('https://sightmap.org/api/atlas/submit', {
    method: 'POST',
    headers: { 'content-type': 'application/x-www-form-urlencoded' },
    body: new URLSearchParams({
      url: 'https://sightmap.org/atlas',
      email: 'chip@sightmap.org',
      ...fields,
    }).toString(),
  })
}

/** Enough of the Netlify Context for this handler: it only reads `ip`. */
const context = { ip: '203.0.113.7' } as unknown as Parameters<typeof handler>[1]

function submissions(): Record<string, unknown>[] {
  return [...store(SUBMISSIONS_STORE).entries()]
    .filter(([key]) => !key.startsWith('index/'))
    .map(([, value]) => value as Record<string, unknown>)
}

let fetchMock: ReturnType<typeof vi.fn>

beforeEach(() => {
  stores.clear()
  etags.clear()
  failing.clear()
  raceOnNextWrite.clear()
  vi.stubEnv('ATLAS_RUNNER', 'netlify')
  vi.stubEnv('NETLIFY_AGENT_TOKEN', 'token')
  vi.stubEnv('NETLIFY_SITE_ID', 'site-1')
  webmcp = () => new Response(CLAIM_FILE, { status: 200 })
  fetchMock = vi.fn(async (input: RequestInfo | URL) => {
    if (String(input).endsWith('/webmcp.txt')) return webmcp()
    return new Response(JSON.stringify({ id: 'run_1', state: 'queued' }), { status: 201 })
  })
  vi.stubGlobal('fetch', fetchMock)
  vi.spyOn(console, 'warn').mockImplementation(() => {})
})

afterEach(() => {
  vi.unstubAllEnvs()
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

describe('POST /api/atlas/submit', () => {
  it('triggers a runner and counts it against the day', async () => {
    const res = await handler(post(), context)
    expect(res.status).toBe(202)
    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(store(RATE_STORE).get(RUNS_KEY)).toEqual({ count: 1 })
    expect(submissions()[0]).toMatchObject({ state: 'new', runner: { kind: 'netlify', id: 'run_1' } })
  })

  it('never spends a run slot when the runner is the queue', async () => {
    vi.stubEnv('ATLAS_RUNNER', 'queue')
    await handler(post(), context)
    expect(fetchMock).not.toHaveBeenCalled()
    expect(store(RATE_STORE).has(RUNS_KEY)).toBe(false)
    expect(submissions()[0]).toMatchObject({ state: 'new', runner: { kind: 'queue' } })
  })

  it('stops triggering at the daily ceiling but still accepts the submission', async () => {
    vi.stubEnv('ATLAS_DAILY_RUNS', '1')
    store(RATE_STORE).set(RUNS_KEY, { count: 1 })

    const res = await handler(post(), context)

    expect(res.status).toBe(202)
    const body = (await res.json()) as { ok: boolean; id: string; state: string; message: string }
    expect(body.ok).toBe(true)
    expect(body.state).toBe('received')
    // The submitter's experience is unchanged — same 202, same message.
    expect(body.message).toContain('Request received.')

    expect(fetchMock).not.toHaveBeenCalled()
    // The ceiling is a ceiling, not a running total: nothing is incremented.
    expect(store(RATE_STORE).get(RUNS_KEY)).toEqual({ count: 1 })

    const record = submissions()[0]!
    expect(record).toMatchObject({
      id: body.id,
      state: 'queued',
      runner: { kind: 'netlify', skipped: 'daily-ceiling' },
    })
    expect(record.runner).not.toHaveProperty('id')
  })

  it('fails open when the run counter cannot be read', async () => {
    failing.add(RATE_STORE)
    const res = await handler(post(), context)
    expect(res.status).toBe(202)
    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(submissions()[0]).toMatchObject({ state: 'new' })
  })

  it('runs nothing at all when ATLAS_DAILY_RUNS is 0, even blind', async () => {
    // 0 is how an operator turns runs off. An unreadable counter is not a
    // reason to start one anyway.
    vi.stubEnv('ATLAS_DAILY_RUNS', '0')
    failing.add(RATE_STORE)
    const res = await handler(post(), context)
    expect(res.status).toBe(202)
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('retries the claim when another invocation wins the write', async () => {
    vi.stubEnv('ATLAS_DAILY_RUNS', '5')
    raceOnNextWrite.set(`${RATE_STORE}/${RUNS_KEY}`, 1)

    const res = await handler(post(), context)

    expect(res.status).toBe(202)
    expect(fetchMock).toHaveBeenCalledTimes(1)
    // The competing write counted 1; the retry read it back and counted 2.
    // A read-then-write would have stored 1 and lost the other run.
    expect(store(RATE_STORE).get(RUNS_KEY)).toEqual({ count: 2 })
    expect(submissions()[0]).toMatchObject({ state: 'new' })
  })

  it('queues rather than overshooting when the race is lost every time', async () => {
    vi.stubEnv('ATLAS_DAILY_RUNS', '50')
    // Never let a conditional write land: three attempts, three conflicts.
    raceOnNextWrite.set(`${RATE_STORE}/${RUNS_KEY}`, RUN_CLAIM_ATTEMPTS)

    const res = await handler(post(), context)

    expect(res.status).toBe(202)
    expect(fetchMock).not.toHaveBeenCalled()
    expect(submissions()[0]).toMatchObject({
      state: 'queued',
      runner: { kind: 'netlify', skipped: 'daily-ceiling' },
    })
  })

  it('counts one submission per run up to the ceiling, then queues the rest', async () => {
    vi.stubEnv('ATLAS_DAILY_RUNS', '2')
    const states: unknown[] = []
    for (let i = 0; i < 3; i += 1) {
      await handler(post(), context)
      const filed = submissions()
      states.push(filed[filed.length - 1]!.state)
    }
    expect(fetchMock).toHaveBeenCalledTimes(2)
    expect(store(RATE_STORE).get(RUNS_KEY)).toEqual({ count: 2 })
    expect(states).toEqual(['new', 'new', 'queued'])
  })

  // -------------------------------------------------------------------------
  // Claims. A claim is checked after the rate-limit window is spent (so the
  // fetch it triggers is metered) and before anything is written, so a host
  // that has not published the line yet can fix it and retry.

  it('verifies a claim, records the date, writes the card record, and answers with the card', async () => {
    const res = await handler(post({ claim: TOKEN }), context)

    expect(res.status).toBe(202)
    const body = (await res.json()) as { id: string; card?: string }
    expect(body.card).toBe(`https://sightmap.org/try/${HOST}`)

    const record = submissions()[0] as { id: string; claim?: { verifiedAt: string } }
    expect(record.claim?.verifiedAt).toMatch(/^\d{4}-\d{2}-\d{2}T/)
    // The token is compared and dropped. Nothing on file may carry it.
    expect(JSON.stringify(record)).not.toContain(TOKEN)

    const card = store(TRY_STORE).get(HOST) as TryRecord
    expect(card).toMatchObject({ v: 1, host: HOST, submissionId: body.id, url: 'https://sightmap.org/atlas' })
    expect(new Date(card.expiresAt).getTime() - new Date(card.claimedAt).getTime()).toBe(30 * 24 * 60 * 60 * 1000)
    expect(card.scan).toBeUndefined()
  })

  it('replaces the card record on a fresh claim and drops the old scan', async () => {
    store(TRY_STORE).set(HOST, {
      v: 1,
      host: HOST,
      url: 'https://sightmap.org/atlas',
      submissionId: 'older',
      claimedAt: '2020-01-01T00:00:00.000Z',
      expiresAt: '2020-01-31T00:00:00.000Z',
      scan: { scannedAt: '2020-01-02T00:00:00.000Z', status: 'tools-found', pages: 2, tools: [] },
    } satisfies TryRecord)

    await handler(post({ claim: TOKEN }), context)

    const card = store(TRY_STORE).get(HOST) as TryRecord
    expect(card.submissionId).not.toBe('older')
    // The old scan described the site before this submission, so it goes.
    expect(card.scan).toBeUndefined()
  })

  it('refuses a claim the host does not carry, and writes nothing but the window', async () => {
    webmcp = () => new Response('https://sightmap.org/\nsearch — Search\n', { status: 200 })
    const res = await handler(post({ claim: TOKEN }), context)

    expect(res.status).toBe(422)
    const body = (await res.json()) as { ok: boolean; error: { code: string; status: number } }
    expect(body).toMatchObject({ ok: false, error: { code: 'claim-mismatch', status: 422 } })
    expect(submissions()).toHaveLength(0)
    expect(store(TRY_STORE).size).toBe(0)
    // The per-IP window is spent (the check made us fetch a host the caller
    // chose), but no day counter and no record: a retry costs one attempt.
    expect([...store(RATE_STORE).keys()].some((k) => k.startsWith('runs/'))).toBe(false)
    expect(store(RATE_STORE).size).toBe(1)
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('does not fetch the claim file for a caller already over the window', async () => {
    webmcp = () => new Response('https://sightmap.org/\nsearch — Search\n', { status: 200 })
    for (let i = 0; i < 6; i += 1) await handler(post({ claim: TOKEN }), context)
    const before = fetchMock.mock.calls.length
    const res = await handler(post({ claim: TOKEN }), context)

    expect(res.status).toBe(429)
    expect(fetchMock.mock.calls.length).toBe(before)
  })

  it('refuses an unreachable claim file the same way', async () => {
    webmcp = () => new Response('not found', { status: 404 })
    const res = await handler(post({ claim: TOKEN }), context)

    expect(res.status).toBe(422)
    expect(((await res.json()) as { error: { code: string } }).error.code).toBe('claim-unreachable')
    expect(submissions()).toHaveLength(0)
  })

  it('rejects a malformed claim token before touching the network', async () => {
    const res = await handler(post({ claim: 'not-a-token' }), context)

    expect(res.status).toBe(400)
    expect(((await res.json()) as { error: { code: string } }).error.code).toBe('claim-invalid')
    expect(fetchMock).not.toHaveBeenCalled()
    expect(submissions()).toHaveLength(0)
  })

  it('carries the card through the no-JS redirect', async () => {
    const res = await handler(formPost({ claim: TOKEN }), context)

    expect(res.status).toBe(303)
    const location = new URL(res.headers.get('location')!, 'https://sightmap.org')
    expect(location.searchParams.get('card')).toBe(`https://sightmap.org/try/${HOST}`)
    expect(location.searchParams.get('submitted')).toMatch(/^[0-9a-f]{16}$/)
  })

  it('sends a rejected claim back to the form with its code', async () => {
    webmcp = () => new Response('nothing here', { status: 200 })
    const res = await handler(formPost({ claim: TOKEN }), context)
    expect(res.status).toBe(303)
    expect(res.headers.get('location')).toBe('/atlas?error=claim-mismatch#submit')
  })

  it('never writes a card record for a submission without a claim', async () => {
    await handler(post(), context)
    expect(store(TRY_STORE).size).toBe(0)
    expect(submissions()[0]).not.toHaveProperty('claim')
    expect((await (await handler(post(), context)).json()) as { card?: string }).not.toHaveProperty('card')
  })

  // -------------------------------------------------------------------------
  // Quarantine. A maintainer sets one key from the CLI; both paths refuse.

  it('refuses a quarantined host, with or without a claim', async () => {
    store(QUARANTINE_STORE).set(HOST, { at: '2026-09-10T00:00:00Z', reason: 'owner request' })

    for (const body of [post(), post({ claim: TOKEN })]) {
      const res = await handler(body, context)
      expect(res.status).toBe(403)
      expect(((await res.json()) as { error: { code: string } }).error.code).toBe('quarantined')
    }
    expect(submissions()).toHaveLength(0)
    expect(store(TRY_STORE).size).toBe(0)
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('accepts submissions when the quarantine store is unreadable', async () => {
    // Fail open: a store that is down must not refuse every submission.
    failing.add(QUARANTINE_STORE)
    const res = await handler(post(), context)
    expect(res.status).toBe(202)
  })

  // -------------------------------------------------------------------------
  // The no-JS path. A browser with scripting off follows a 303; it cannot
  // read a JSON error envelope, so every answer to a form post is a redirect
  // back to the form, and JSON clients still get JSON.

  it('redirects a form post back to the form with the new id', async () => {
    const res = await handler(formPost(), context)
    expect(res.status).toBe(303)
    const location = res.headers.get('location')!
    expect(location).toMatch(/^\/atlas\?submitted=[0-9a-f]{16}#submit$/)
    expect(res.headers.get('cache-control')).toBe('no-store')
    const id = new URL(location, 'https://sightmap.org').searchParams.get('submitted')
    expect(submissions()[0]).toMatchObject({ id, state: 'new' })
  })

  it('redirects a rejected form post back to the form with the error code', async () => {
    const res = await handler(formPost({ url: 'http://sightmap.org/atlas' }), context)
    expect(res.status).toBe(303)
    expect(res.headers.get('location')).toBe('/atlas?error=invalid_url#submit')
    expect(submissions()).toHaveLength(0)
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('redirects a rate-limited form post instead of answering JSON', async () => {
    for (let i = 0; i < 5; i += 1) await handler(formPost(), context)
    const res = await handler(formPost(), context)
    expect(res.status).toBe(303)
    expect(res.headers.get('location')).toBe('/atlas?error=rate_limited#submit')
  })

  it('keeps answering JSON clients with JSON', async () => {
    for (let i = 0; i < 5; i += 1) await handler(post(), context)
    const limited = await handler(post(), context)
    expect(limited.status).toBe(429)
    expect(limited.headers.get('content-type')).toContain('application/json')
    expect(((await limited.json()) as { error: { code: string } }).error.code).toBe('rate_limited')

    const invalid = await handler(post({ url: 'http://sightmap.org/atlas' }), { ip: '198.51.100.4' } as unknown as Parameters<typeof handler>[1])
    expect(invalid.status).toBe(400)
    expect(((await invalid.json()) as { error: { code: string } }).error.code).toBe('invalid_url')
  })
})
