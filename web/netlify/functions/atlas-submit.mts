// POST /api/atlas/submit — the Atlas submission endpoint.
//
// A thin wrapper. Everything worth testing lives in ../lib/submit.ts (parsing,
// validation, the record) and ../lib/runner.ts (the daily ceiling, the prompt,
// the two runner payloads), each covered by its own test file; this file only
// wires them to the Netlify runtime: the Request, two Blobs stores, fetch, and
// the environment.
//
// Two invariants the code below is built around:
//
//   1. A submission is never lost to storage. Blobs is wrapped so that a read
//      or write failure degrades to "accepted but not persisted" with a logged
//      warning — never a 500. The same goes for the runner: a trigger failure
//      is recorded on the submission and still answered 202.
//   2. The email address never leaves this function except into the
//      `atlas-submissions` record. Not into the prompt, the GitHub payload, a
//      log line, or the status response. Only `emailHash` travels.
//
// Environment variables
// ---------------------
//   ATLAS_RUNNER               'netlify' (primary), 'github', or 'queue'/unset.
//                              'queue' stores the submission and stops; a
//                              maintainer runs the intake by hand.
//   NETLIFY_AGENT_TOKEN        Netlify personal access token, for ATLAS_RUNNER=netlify.
//                              Used against POST /api/v1/agent_runners — the
//                              call `netlify agents:create` makes.
//   NETLIFY_SITE_ID            Site to create the agent runner in. Falls back to
//                              SITE_ID, which Netlify injects at runtime.
//   ATLAS_RUNNER_BRANCH_BASE   Optional. Branch the runner starts from; omitted
//                              from the request body when unset.
//   ATLAS_RUNNER_MODEL         Optional. Model for the runner; omitted when unset.
//   ATLAS_DAILY_RUNS           Optional, default 20. Global ceiling on runner
//                              triggers per UTC day, across every submitter.
//                              Agent Runner concurrency is capped per plan, so
//                              a burst from distinct IPs (which the per-IP
//                              limit does not catch) must not exhaust it. Over
//                              the ceiling the submission is still accepted and
//                              stored, with state 'queued' and no runner.
//   ATLAS_GITHUB_TOKEN         Token for ATLAS_RUNNER=github (repository_dispatch).
//   ATLAS_GITHUB_REPO          Optional. Defaults to 'sightmap/sightmap'.
//   ATLAS_SUBMIT_SALT          Per-deploy salt for the client IP hash. Defaults
//                              to 'atlas'; set it so hashes are not comparable
//                              across deploys.
//
// Blobs stores: `atlas-submissions` (the records) and `atlas-rate` (one counter
// per hashed IP, plus one `runs/<yyyy-mm-dd>` counter for the daily ceiling).

import { getStore } from '@netlify/blobs'
import type { Context } from '@netlify/functions'
import {
  acceptedBody,
  buildRecord,
  hashEmail,
  hashIp,
  indexKey,
  newSubmissionId,
  parseBody,
  parseRateState,
  publicStatus,
  rateDecision,
  rateLimitedError,
  recordKey,
  submitError,
  validateSubmission,
  type SubmissionRecord,
  type SubmitErrorBody,
} from '../lib/submit.ts'
import {
  CEILING_SKIP_REASON,
  dailyRunCeiling,
  dailyRunsKey,
  parseRunCount,
  runnerInput,
  runnerKind,
  shouldTriggerRunner,
  submitSalt,
  triggerRunner,
  type RunnerEnv,
} from '../lib/runner.ts'
import { methodNotAllowedError, notFoundError } from '../lib/errors.ts'

export const config = {
  // The request chain, in full — netlify.toml, netlify/lib/handler.ts and
  // src/lib/submit-types.ts all point here rather than repeat it. Netlify
  // evaluates functions before redirects, so a function path wins over both the
  // /api/atlas/:slug rewrite and the /* → 404 catch-all in netlify.toml, and
  // /api/atlas/submit reaches this handler with no rule of its own. Declaring a
  // custom path normally *removes* the default /.netlify/functions/<name> URL,
  // so that one is listed too and stays available as an alias.
  path: ['/api/atlas/submit', '/.netlify/functions/atlas-submit'],
}

const JSON_TYPE = 'application/json; charset=utf-8'
const SUBMISSIONS_STORE = 'atlas-submissions'
const RATE_STORE = 'atlas-rate'

function json(body: unknown, status: number, extra?: HeadersInit): Response {
  const headers = new Headers(extra)
  headers.set('Content-Type', JSON_TYPE)
  // No Access-Control-Allow-Origin here: this endpoint is same-origin only.
  // netlify.toml already sets `*` for /api/* on the static API files; that is
  // read-only data and deliberately left alone. No preflight handling either —
  // a simple form post does not need one, and a cross-origin fetch should fail.
  headers.set('Cache-Control', 'no-store')
  return new Response(JSON.stringify(body), { status, headers })
}

function errorResponse(body: SubmitErrorBody, extra?: HeadersInit): Response {
  return json(body, body.error.status, extra)
}

/** Blobs is best-effort everywhere. A failure is a warning, never a 500. */
async function withStore<T>(
  name: string,
  what: string,
  fn: (store: ReturnType<typeof getStore>) => Promise<T>
): Promise<{ ok: true; value: T } | { ok: false; error: string }> {
  try {
    return { ok: true, value: await fn(getStore(name)) }
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err)
    console.warn(`[atlas-submit] blobs ${what} failed on store ${name}: ${message}`)
    return { ok: false, error: message }
  }
}

async function handleGet(url: URL): Promise<Response> {
  const id = (url.searchParams.get('id') ?? '').trim()
  if (!id || !/^[A-Za-z0-9_-]{1,64}$/.test(id)) {
    return errorResponse(
      submitError(
        'invalid_id',
        'Pass the submission id returned when the submission was accepted.',
        'GET /api/atlas/submit?id=<id>',
        400
      )
    )
  }

  const found = await withStore(SUBMISSIONS_STORE, 'status lookup', async (store) => {
    const pointer = (await store.get(indexKey(id), { type: 'json' })) as { key?: string } | null
    if (!pointer?.key) return null
    return (await store.get(pointer.key, { type: 'json' })) as SubmissionRecord | null
  })

  if (!found.ok || !found.value) {
    return json({ ok: false, error: notFoundError(`/api/atlas/submit?id=${id}`).error }, 404)
  }
  return json(publicStatus(found.value), 200)
}

export default async (req: Request, context: Context): Promise<Response> => {
  const url = new URL(req.url)

  if (req.method === 'GET' || req.method === 'HEAD') return handleGet(url)
  if (req.method !== 'POST') {
    return json(
      { ok: false, error: methodNotAllowedError(req.method, url.pathname).error },
      405,
      { Allow: 'GET, POST' }
    )
  }

  const parsed = parseBody(req.headers.get('content-type'), await req.text())
  if (!parsed.ok) return errorResponse(parsed.error)
  const { fields, encoding } = parsed

  const validated = await validateSubmission(fields)
  if (!validated.ok) return errorResponse(validated.error)
  const value = validated.value

  const env = process.env as RunnerEnv
  const ipHash = await hashIp(context.ip ?? 'unknown', submitSalt(env))

  // Rate limit. A Blobs failure here fails open: five extra submissions cost
  // far less than one lost one.
  const now = Date.now()
  const rate = await withStore(RATE_STORE, 'rate limit', async (store) => {
    const previous = parseRateState(await store.get(ipHash, { type: 'json' }))
    const decision = rateDecision(previous, now)
    if (decision.allowed) await store.setJSON(ipHash, decision.state)
    return decision
  })
  if (rate.ok && !rate.value.allowed) {
    return errorResponse(rateLimitedError(rate.value.retryAfterSeconds), {
      'Retry-After': String(rate.value.retryAfterSeconds),
    })
  }

  const id = newSubmissionId()
  const receivedAt = new Date(now).toISOString()
  const emailHash = await hashEmail(value.email)
  const kind = runnerKind(env)

  // Global daily ceiling on runner triggers. Claimed only when we are actually
  // about to start a run, so the 'queue' runner (which starts nothing) never
  // spends a slot. Same fail-open rule as the rate limit: a Blobs failure lets
  // the run through rather than stalling the pipeline.
  const runsKey = dailyRunsKey(receivedAt)
  const claim =
    kind === 'queue'
      ? { ok: true as const, value: true }
      : await withStore(RATE_STORE, 'daily run ceiling', async (store) => {
          const count = parseRunCount(await store.get(runsKey, { type: 'json' }))
          const allowed = shouldTriggerRunner(count, dailyRunCeiling(env))
          if (allowed) await store.setJSON(runsKey, { count: (count ?? 0) + 1 })
          return allowed
        })
  const mayRun = claim.ok ? claim.value : true

  const record = buildRecord(value, {
    id,
    receivedAt,
    emailHash,
    ipHash,
    userAgent: req.headers.get('user-agent') ?? '',
    runner: { kind },
    ...(mayRun ? {} : { state: 'queued' as const }),
  })

  if (mayRun) {
    // Trigger before the write so the runner id lands in the stored record.
    // Never throws: a failure comes back as runner.error.
    record.runner = await triggerRunner(runnerInput(record), env)
    if (record.runner.error) {
      console.warn(`[atlas-submit] runner ${record.runner.kind} failed for ${id}: ${record.runner.error}`)
    }
  } else {
    // Accepted, stored, and left for a maintainer or for tomorrow. The
    // submitter sees the same 202 and the same message either way.
    record.runner = { kind, skipped: CEILING_SKIP_REASON }
    console.warn(
      `[atlas-submit] daily runner ceiling (${dailyRunCeiling(env)}) reached; queued ${id} host=${record.host}`
    )
  }

  const key = recordKey(receivedAt, id)
  const stored = await withStore(SUBMISSIONS_STORE, 'persist', async (store) => {
    await store.setJSON(key, record)
    // Pointer blob so the status lookup is one read. Written second: if it
    // fails the submission is still on file, only ?id= degrades to a 404.
    await store.setJSON(indexKey(id), { key })
    return true
  })
  if (!stored.ok) {
    // Accepted but not persisted. The log line is the record of last resort —
    // host and id only, never the email.
    console.error(
      `[atlas-submit] NOT PERSISTED id=${id} host=${record.host} runner=${record.runner.kind} intent=${record.intent ? 'yes' : 'no'}`
    )
  }

  if (encoding === 'form') {
    // No-JS path: back to the page that posted, with the id in the query so it
    // can render a confirmation.
    return new Response(null, {
      status: 303,
      headers: { Location: `/atlas?submitted=${encodeURIComponent(id)}`, 'Cache-Control': 'no-store' },
    })
  }

  return json(acceptedBody(id), 202)
}
