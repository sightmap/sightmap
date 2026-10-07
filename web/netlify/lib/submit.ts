// Pure logic for the Atlas submission endpoint: parsing, validation, hashing,
// the stored record, and the per-IP rate limit. The runner half — the daily
// ceiling, the prompt, and the two trigger payloads — lives in `runner.ts`.
//
// Everything here is deterministic and dependency-injected. The function file
// (`netlify/functions/atlas-submit.mts`) is a thin wrapper that supplies the
// request, the Blobs stores, `fetch`, and `process.env`. That split is what
// lets the interesting behaviour be tested in plain vitest, with no Netlify
// runtime and no network.
//
// One rule runs through both files: the submitter's email address is stored in
// exactly one place — the submission record in Blobs. It never reaches the
// runner prompt, the GitHub dispatch payload, a branch name, a PR, or a log
// line. Only `emailHash` travels.

import { apiError, type ApiErrorBody } from './errors.ts'
import { CLAIM_TOKEN_RE, type ClaimFailure } from './claim.ts'
import { expiresAfter, type TryRecord } from './try-record.ts'
import { preflightUrl, resolvePublic } from '../../scripts/lib/preflight.ts'
import { canonicalHost } from '../../scripts/lib/directory.ts'
import { SITE_URL } from '../../scripts/lib/site.ts'
import {
  MAX_EMAIL_LENGTH,
  MAX_INTENT_LENGTH,
  RATE_LIMIT_MAX,
  RATE_LIMIT_WINDOW_MS,
  type RunnerKind,
  type SubmitAccepted,
  type SubmitState,
  type SubmitStatus,
} from '../../src/lib/submit-types.ts'

export const MAX_LOCAL_PART_LENGTH = 64

export const ACCEPTED_MESSAGE =
  'Request received. We normally scan submissions within one business day and email the report.'

const SUBMIT_HINT =
  'POST JSON or form fields { url, email, owner?, sightkick?, intent?, nominate?, rescan?, claim? } to /api/atlas/submit. See https://sightmap.org/atlas.'

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

/**
 * The endpoint's error envelope. `error` is the same object shape the rest of
 * the JSON API returns (`netlify/lib/errors.ts`), wrapped in the `ok: false`
 * discriminator the form's fetch caller switches on.
 */
export interface SubmitErrorBody {
  ok: false
  error: ApiErrorBody['error']
}

export function submitError(
  code: string,
  message: string,
  hint: string,
  status: number
): SubmitErrorBody {
  return { ok: false, error: apiError(code, message, hint, status).error }
}

export function badRequest(code: string, message: string, hint = SUBMIT_HINT): SubmitErrorBody {
  return submitError(code, message, hint, 400)
}

const CLAIM_HINT =
  'Serve https://<host>/webmcp.txt with a line `# sightmap-claim: <token>`, where <token> is the 32 hex characters you sent as `claim`, then submit again.'

/**
 * A claim that could not be checked. 422, not 400: the request was well
 * formed, the host just does not (yet) carry the line. The message is fixed
 * per code and never carries the reason: the check makes this site fetch a
 * URL the caller chose, and echoing the upstream status or the failure mode
 * would turn it into a probe of that host. The reason goes to the log.
 */
export function claimRejectedError(code: ClaimFailure): SubmitErrorBody {
  const message =
    code === 'claim-mismatch'
      ? 'webmcp.txt was read but does not carry a matching claim line.'
      : 'webmcp.txt could not be read from that host over https.'
  return submitError(code, message, CLAIM_HINT, 422)
}

/** A host a maintainer has taken off the pipeline. Same answer with or without a claim. */
export function quarantinedError(host: string): SubmitErrorBody {
  return submitError(
    'quarantined',
    `Submissions for ${host} are not being accepted.`,
    'Email hello@sightmap.org if you think this is a mistake.',
    403
  )
}

/** Where a verified claim gets its unlisted card. Not a listing, and not indexed. */
export function cardUrl(host: string): string {
  return `${SITE_URL}/try/${canonicalHost(host)}`
}

// ---------------------------------------------------------------------------
// Parsing
// ---------------------------------------------------------------------------

export interface SubmissionFields {
  url: string
  email: string
  owner: boolean
  sightkick: boolean
  intent: string
  nominate: boolean
  rescan: boolean
  /** Optional domain-control token; see `claim.ts`. Never stored. */
  claim: string
  website: string
}

/** How the body arrived — a JSON fetch, or a plain `<form method="post">`. */
export type BodyEncoding = 'json' | 'form'

const TRUTHY = new Set(['true', '1', 'on', 'yes', 'y'])

function asString(value: unknown): string {
  if (typeof value === 'string') return value.trim()
  if (typeof value === 'number' && Number.isFinite(value)) return String(value)
  return ''
}

function asBoolean(value: unknown): boolean {
  if (typeof value === 'boolean') return value
  if (typeof value === 'number') return value === 1
  if (typeof value === 'string') return TRUTHY.has(value.trim().toLowerCase())
  return false
}

/** Pull the known fields out of a decoded body. Unknown keys are ignored. */
export function readFields(input: Record<string, unknown>): SubmissionFields {
  return {
    url: asString(input.url),
    email: asString(input.email),
    owner: asBoolean(input.owner),
    sightkick: asBoolean(input.sightkick),
    intent: asString(input.intent),
    nominate: asBoolean(input.nominate),
    rescan: asBoolean(input.rescan),
    claim: asString(input.claim).toLowerCase(),
    website: asString(input.website),
  }
}

/** `application/x-www-form-urlencoded` → a plain object. Last value wins. */
export function fieldsFromForm(body: string): SubmissionFields {
  const params = new URLSearchParams(body)
  const raw: Record<string, unknown> = {}
  for (const [key, value] of params) raw[key] = value
  return readFields(raw)
}

/**
 * Decode a request body into fields plus the encoding that produced it (the
 * encoding decides whether we answer with JSON or a 303 back to /atlas).
 * Returns an error body rather than throwing: a malformed post is a 400.
 */
export function parseBody(
  contentType: string | null,
  body: string
): { ok: true; fields: SubmissionFields; encoding: BodyEncoding } | { ok: false; error: SubmitErrorBody } {
  const type = (contentType ?? '').split(';')[0]!.trim().toLowerCase()

  if (type === 'application/x-www-form-urlencoded') {
    return { ok: true, fields: fieldsFromForm(body), encoding: 'form' }
  }

  if (type === '' || type === 'application/json' || type === 'text/plain') {
    let parsed: unknown
    try {
      parsed = JSON.parse(body)
    } catch {
      return { ok: false, error: badRequest('invalid_body', 'Request body is not valid JSON.') }
    }
    if (parsed === null || typeof parsed !== 'object' || Array.isArray(parsed)) {
      return { ok: false, error: badRequest('invalid_body', 'Request body must be a JSON object.') }
    }
    return { ok: true, fields: readFields(parsed as Record<string, unknown>), encoding: 'json' }
  }

  return {
    ok: false,
    error: badRequest(
      'unsupported_media_type',
      `Content-Type ${type} is not supported. Use application/json or application/x-www-form-urlencoded.`
    ),
  }
}

// ---------------------------------------------------------------------------
// Validation
// ---------------------------------------------------------------------------

// Deliberately narrower than RFC 5321: no quoted local parts, no comments, no
// address literals. Anything this rejects is something we could not email a
// report to anyway.
const EMAIL_RE = /^[A-Za-z0-9!#$%&'*+/=?^_`{|}~-]+(?:\.[A-Za-z0-9!#$%&'*+/=?^_`{|}~-]+)*@[A-Za-z0-9](?:[A-Za-z0-9-]*[A-Za-z0-9])?(?:\.[A-Za-z0-9](?:[A-Za-z0-9-]*[A-Za-z0-9])?)+$/

export function isValidEmail(email: string): boolean {
  if (!email || email.length > MAX_EMAIL_LENGTH) return false
  if (/[\s\u0000-\u001f\u007f]/.test(email)) return false
  const at = email.lastIndexOf('@')
  if (at <= 0 || at > MAX_LOCAL_PART_LENGTH) return false
  return EMAIL_RE.test(email)
}

/** Lowercased for hashing so the same mailbox always yields the same hash. */
export function normalizeEmail(email: string): string {
  return email.trim().toLowerCase()
}

/**
 * Characters an intent has no use for and a shell, a prompt, or a quoted
 * command line does: `$ ` \ " ' ! ( ) | ; & < > { }`. The intent is prose that
 * ends up inside a single-quoted argument in `runner.ts`'s `intakeCommand`;
 * that quoting is the real defence, and this is the second layer, so that a
 * value which escapes one of them is inert in the other.
 */
const INTENT_UNSAFE = /[$`\\"'!()|;&<>{}]/g

/**
 * Collapse submitted free text to a single safe line. Control characters and
 * newlines are removed before the intent is ever interpolated into a prompt or
 * a JSON payload, so a submitter cannot inject prompt structure with a newline,
 * and shell metacharacters go with them — none of them are needed to say what
 * an agent should be able to do.
 */
export function sanitizeIntent(intent: string): string {
  return intent
    .replace(/[\u0000-\u001f\u007f]+/g, ' ')
    .replace(INTENT_UNSAFE, ' ')
    .replace(/\s+/g, ' ')
    .trim()
}

export interface ValidSubmission {
  url: string
  host: string
  email: string
  emailNormalized: string
  owner: boolean
  sightkick: boolean
  intent: string
  nominate: boolean
  rescan: boolean
  /** '' when the submitter did not claim the host. */
  claim: string
}

export interface ValidateDeps {
  /** Injected in tests; defaults to the real DNS resolver in `preflight.ts`. */
  lookup?: (host: string) => Promise<{ address: string }[]>
}

/**
 * Full validation of one submission: honeypot, URL (syntactic then DNS),
 * email, intent length. The URL is checked with the same `preflightUrl` the
 * scanner uses and never with `allowLocal` — a submitted address must be
 * public before we point a browser at it — and then resolved so that
 * `internal.example.com → 10.0.0.5` is caught too.
 */
export async function validateSubmission(
  fields: SubmissionFields,
  deps: ValidateDeps = {}
): Promise<{ ok: true; value: ValidSubmission } | { ok: false; error: SubmitErrorBody }> {
  if (fields.website) {
    // Honeypot. Say nothing useful; a bot gets the same answer as a typo.
    return { ok: false, error: badRequest('rejected', 'Submission rejected.') }
  }

  const pre = preflightUrl(fields.url)
  if (!pre.ok) {
    return {
      ok: false,
      error: badRequest('invalid_url', `That URL cannot be scanned: ${pre.reason}.`),
    }
  }

  const resolved = await resolvePublic(pre.host, deps.lookup)
  if (!resolved.ok) {
    return {
      ok: false,
      error: badRequest('invalid_host', `That host cannot be scanned: ${resolved.reason}.`),
    }
  }

  if (!isValidEmail(fields.email)) {
    return {
      ok: false,
      error: badRequest(
        'invalid_email',
        fields.email.length > MAX_EMAIL_LENGTH
          ? `Email address is longer than ${MAX_EMAIL_LENGTH} characters.`
          : 'Enter an email address we can send the report to.'
      ),
    }
  }

  const intent = sanitizeIntent(fields.intent)
  if (intent.length > MAX_INTENT_LENGTH) {
    return {
      ok: false,
      error: badRequest(
        'intent_too_long',
        `Tell us what an agent should do in ${MAX_INTENT_LENGTH} characters or fewer.`
      ),
    }
  }

  // Shape only. Whether the host actually carries the token is a network
  // check the caller makes after this, so that a typo costs a 400 and not a
  // request to someone else's server.
  if (fields.claim && !CLAIM_TOKEN_RE.test(fields.claim)) {
    return {
      ok: false,
      error: badRequest(
        'claim-invalid',
        'A claim token is 32 lowercase hex characters, the same value as the one in webmcp.txt.'
      ),
    }
  }

  return {
    ok: true,
    value: {
      url: pre.url,
      host: pre.host,
      email: fields.email,
      emailNormalized: normalizeEmail(fields.email),
      owner: fields.owner,
      sightkick: fields.sightkick,
      intent,
      nominate: fields.nominate,
      rescan: fields.rescan,
      claim: fields.claim,
    },
  }
}

// ---------------------------------------------------------------------------
// Hashing
// ---------------------------------------------------------------------------

function toHex(buffer: ArrayBuffer): string {
  return [...new Uint8Array(buffer)].map((b) => b.toString(16).padStart(2, '0')).join('')
}

/** WebCrypto SHA-256, hex encoded. Present in Node 20+ and in the Netlify runtime. */
export async function sha256Hex(input: string): Promise<string> {
  const bytes = new TextEncoder().encode(input)
  return toHex(await crypto.subtle.digest('SHA-256', bytes))
}

/** WebCrypto HMAC-SHA-256, hex encoded. Same availability as `sha256Hex`. */
export async function hmacSha256Hex(key: string, message: string): Promise<string> {
  const encoder = new TextEncoder()
  const cryptoKey = await crypto.subtle.importKey(
    'raw',
    encoder.encode(key),
    { name: 'HMAC', hash: 'SHA-256' },
    false,
    ['sign']
  )
  return toHex(await crypto.subtle.sign('HMAC', cryptoKey, encoder.encode(message)))
}

/** Per-deploy salted client hash. The raw IP is never stored or logged. */
export function hashIp(ip: string, salt: string): Promise<string> {
  return sha256Hex(`${salt}:${ip}`)
}

/**
 * Keyed, not just digested. A plain SHA-256 of an address is reversible for
 * anyone who can guess it — the whole space of "is chip@example.com in your
 * submissions" is one hash per guess — so the hash is an HMAC keyed with
 * `ATLAS_SUBMIT_SALT`, which never leaves the deploy's environment.
 *
 * The cost is that the key is what makes two hashes comparable: change
 * `ATLAS_SUBMIT_SALT`, or run two deploys with different values, and the same
 * mailbox no longer matches itself across them. That is deliberate — it is also
 * what stops the runner's `email_hash` from being correlated with anything
 * outside this deploy — but it means the salt should be set once and left
 * alone, and rotating it retires every hash already on file.
 */
export function hashEmail(email: string, salt: string): Promise<string> {
  return hmacSha256Hex(salt, normalizeEmail(email))
}

// ---------------------------------------------------------------------------
// Record
// ---------------------------------------------------------------------------

export interface RunnerInfo {
  kind: RunnerKind
  id?: string
  state?: string
  error?: string
  /** Set when the trigger was deliberately not made, e.g. 'daily-ceiling'. */
  skipped?: string
}

export interface SubmissionRecord {
  id: string
  receivedAt: string
  url: string
  host: string
  /** The only copy of the submitter's address anywhere in the system. */
  email: string
  emailHash: string
  owner: boolean
  sightkick: boolean
  intent: string
  nominate: boolean
  rescan: boolean
  ipHash: string
  userAgent: string
  state: SubmitState
  runner: RunnerInfo
  /**
   * Present only when the submitter proved control of the host. The token
   * itself is deliberately absent: it is compared during the request and
   * never written anywhere.
   */
  claim?: { verifiedAt: string }
}

export interface RecordMeta {
  id: string
  receivedAt: string
  emailHash: string
  ipHash: string
  userAgent: string
  runner: RunnerInfo
  /** Defaults to 'new'. `queued` means no runner was triggered for it. */
  state?: SubmitState
  claim?: { verifiedAt: string }
}

export function buildRecord(value: ValidSubmission, meta: RecordMeta): SubmissionRecord {
  return {
    id: meta.id,
    receivedAt: meta.receivedAt,
    url: value.url,
    host: value.host,
    email: value.email,
    emailHash: meta.emailHash,
    owner: value.owner,
    sightkick: value.sightkick,
    intent: value.intent,
    nominate: value.nominate,
    rescan: value.rescan,
    ipHash: meta.ipHash,
    userAgent: meta.userAgent.slice(0, 300),
    state: meta.state ?? 'new',
    runner: meta.runner,
    ...(meta.claim ? { claim: meta.claim } : {}),
  }
}

/**
 * The record behind the card at /try/<host>. Written on every verified claim,
 * replacing whatever was there: a fresh claim starts a fresh 30 days and drops
 * the previous scan, which described a site as it was before this submission.
 */
export function buildTryRecord(value: ValidSubmission, meta: { id: string; claimedAt: string }): TryRecord {
  return {
    v: 1,
    host: canonicalHost(value.host),
    url: value.url,
    submissionId: meta.id,
    claimedAt: meta.claimedAt,
    expiresAt: expiresAfter(meta.claimedAt),
  }
}

/** A short, URL-safe id. `crypto.randomUUID` without the dashes, truncated. */
export function newSubmissionId(uuid: string = crypto.randomUUID()): string {
  return uuid.replace(/-/g, '').slice(0, 16)
}

/** Records are filed by day so a maintainer can list one day's intake. */
export function recordKey(receivedAt: string, id: string): string {
  return `${receivedAt.slice(0, 10)}/${id}`
}

/**
 * A pointer blob so `GET ?id=` is one read instead of a listing. If this write
 * fails the submission is still stored and still runs; only the status lookup
 * degrades to a 404.
 */
export function indexKey(id: string): string {
  return `index/${id}`
}

/** What `GET ?id=` returns: no email, and no URL beyond the host. */
export function publicStatus(record: SubmissionRecord): SubmitStatus {
  return {
    ok: true,
    id: record.id,
    state: record.state,
    receivedAt: record.receivedAt,
    host: record.host,
    runner: { kind: record.runner.kind, ...(record.runner.state ? { state: record.runner.state } : {}) },
  }
}

export function acceptedBody(id: string, card?: string): SubmitAccepted {
  return { ok: true, id, state: 'received', message: ACCEPTED_MESSAGE, ...(card ? { card } : {}) }
}

// ---------------------------------------------------------------------------
// Rate limiting
// ---------------------------------------------------------------------------

export interface RateState {
  count: number
  resetAt: number
}

export interface RateDecision {
  allowed: boolean
  state: RateState
  retryAfterSeconds: number
}

/**
 * Fixed window per hashed IP. Pure, so the storage layer can be a Blobs read
 * that is allowed to fail: on a read failure the caller passes `null` and the
 * submitter is let through rather than losing the submission.
 */
export function rateDecision(
  previous: RateState | null,
  now: number,
  limit = RATE_LIMIT_MAX,
  windowMs = RATE_LIMIT_WINDOW_MS
): RateDecision {
  if (!previous || now >= previous.resetAt) {
    return { allowed: true, state: { count: 1, resetAt: now + windowMs }, retryAfterSeconds: 0 }
  }
  if (previous.count >= limit) {
    return {
      allowed: false,
      state: previous,
      retryAfterSeconds: Math.max(1, Math.ceil((previous.resetAt - now) / 1000)),
    }
  }
  return {
    allowed: true,
    state: { count: previous.count + 1, resetAt: previous.resetAt },
    retryAfterSeconds: 0,
  }
}

export function rateLimitedError(retryAfterSeconds: number): SubmitErrorBody {
  const hours = Math.max(1, Math.round(retryAfterSeconds / 3600))
  return submitError(
    'rate_limited',
    `Too many submissions from this network. Try again in about ${hours} hour${hours === 1 ? '' : 's'}.`,
    `The limit is ${RATE_LIMIT_MAX} submissions per 24 hours. Email hello@sightmap.org if you need to submit more.`,
    429
  )
}

/** Parse whatever came back from Blobs; anything unexpected is "no history". */
export function parseRateState(value: unknown): RateState | null {
  if (!value || typeof value !== 'object') return null
  const raw = value as Record<string, unknown>
  const count = typeof raw.count === 'number' ? raw.count : Number.NaN
  const resetAt = typeof raw.resetAt === 'number' ? raw.resetAt : Number.NaN
  if (!Number.isFinite(count) || !Number.isFinite(resetAt)) return null
  return { count, resetAt }
}
