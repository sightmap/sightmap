// Function-level tests for the Atlas submission endpoint.
//
// Everything deterministic lives in ../lib/submit.ts and ../lib/runner.ts and is
// tested there. What is left here is the wiring the pure functions cannot see:
// which Blobs keys the handler touches, and the branch where the global daily
// runner ceiling is reached — the submission is still stored and still answered
// 202, but no runner is triggered.
//
// Two fakes stand in for the runtime: an in-memory Blobs store keyed by store
// name, and a stubbed global fetch (the runner trigger). DNS is stubbed too so
// `resolvePublic` never leaves the machine.

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import handler from './atlas-submit.mts'

/** In-memory Blobs: `stores.get('atlas-rate')` is one store's key → value map. */
const stores = new Map<string, Map<string, unknown>>()
/** Store names whose next operation should throw, to exercise the fail-open paths. */
const failing = new Set<string>()

function store(name: string): Map<string, unknown> {
  let s = stores.get(name)
  if (!s) {
    s = new Map()
    stores.set(name, s)
  }
  return s
}

vi.mock('@netlify/blobs', () => ({
  getStore: (name: string) => ({
    async get(key: string) {
      if (failing.has(name)) throw new Error(`blobs unavailable: ${name}`)
      return store(name).get(key) ?? null
    },
    async setJSON(key: string, value: unknown) {
      if (failing.has(name)) throw new Error(`blobs unavailable: ${name}`)
      store(name).set(key, value)
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

function post(body: Record<string, unknown> = {}): Request {
  return new Request('https://sightmap.org/api/atlas/submit', {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ url: 'https://sightmap.org/atlas', email: 'chip@sightmap.org', ...body }),
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
  failing.clear()
  vi.stubEnv('ATLAS_RUNNER', 'netlify')
  vi.stubEnv('NETLIFY_AGENT_TOKEN', 'token')
  vi.stubEnv('NETLIFY_SITE_ID', 'site-1')
  fetchMock = vi.fn(async () => new Response(JSON.stringify({ id: 'run_1', state: 'queued' }), { status: 201 }))
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
    vi.stubEnv('ATLAS_DAILY_RUNS', '0')
    failing.add(RATE_STORE)
    const res = await handler(post(), context)
    expect(res.status).toBe(202)
    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(submissions()[0]).toMatchObject({ state: 'new' })
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
})
