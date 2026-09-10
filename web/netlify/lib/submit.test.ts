import { describe, expect, it, vi } from 'vitest'
import { MAX_INTENT_LENGTH } from '../../src/lib/submit-types.ts'
import {
  acceptedBody,
  buildRecord,
  buildTryRecord,
  cardUrl,
  claimRejectedError,
  fieldsFromForm,
  hashEmail,
  hashIp,
  hmacSha256Hex,
  indexKey,
  isValidEmail,
  newSubmissionId,
  parseBody,
  parseRateState,
  publicStatus,
  quarantinedError,
  rateDecision,
  rateLimitedError,
  readFields,
  recordKey,
  sanitizeIntent,
  sha256Hex,
  validateSubmission,
  type SubmissionFields,
  type ValidSubmission,
} from './submit'

const EMAIL = 'chip@example.com'

function fields(overrides: Partial<SubmissionFields> = {}): SubmissionFields {
  return {
    url: 'https://example.org/pricing',
    email: EMAIL,
    owner: false,
    sightkick: false,
    intent: '',
    nominate: false,
    rescan: false,
    claim: '',
    website: '',
    ...overrides,
  }
}

/** A resolver that answers every host with one public address. */
const publicLookup = async () => [{ address: '93.184.216.34' }]
const privateLookup = async () => [{ address: '10.0.0.5' }]

async function validate(overrides: Partial<SubmissionFields> = {}, lookup = publicLookup) {
  return validateSubmission(fields(overrides), { lookup })
}

// ---------------------------------------------------------------------------

describe('parseBody', () => {
  it('reads a JSON post', () => {
    const result = parseBody('application/json', JSON.stringify({ url: 'https://a.example', email: EMAIL, owner: true }))
    expect(result.ok).toBe(true)
    if (!result.ok) return
    expect(result.encoding).toBe('json')
    expect(result.fields.url).toBe('https://a.example')
    expect(result.fields.owner).toBe(true)
    expect(result.fields.sightkick).toBe(false)
  })

  it('reads a no-JS form post, including checkbox "on"', () => {
    const body = new URLSearchParams({
      url: ' https://a.example ',
      email: EMAIL,
      sightkick: 'on',
      nominate: 'true',
      intent: 'Book a table',
    }).toString()
    const result = parseBody('application/x-www-form-urlencoded; charset=UTF-8', body)
    expect(result.ok).toBe(true)
    if (!result.ok) return
    expect(result.encoding).toBe('form')
    expect(result.fields.url).toBe('https://a.example')
    expect(result.fields.sightkick).toBe(true)
    expect(result.fields.nominate).toBe(true)
    expect(result.fields.rescan).toBe(false)
    expect(result.fields.intent).toBe('Book a table')
  })

  it('400s on a body that is not JSON, and on a JSON array', () => {
    const broken = parseBody('application/json', '{oops')
    expect(broken.ok).toBe(false)
    if (broken.ok) return
    expect(broken.error.ok).toBe(false)
    expect(broken.error.error.code).toBe('invalid_body')
    expect(broken.error.error.status).toBe(400)

    const array = parseBody('application/json', '[]')
    expect(array.ok).toBe(false)
  })

  it('400s on an unsupported content type', () => {
    const result = parseBody('multipart/form-data; boundary=x', 'whatever')
    expect(result.ok).toBe(false)
    if (result.ok) return
    expect(result.error.error.code).toBe('unsupported_media_type')
  })

  it('ignores unknown fields', () => {
    expect(readFields({ url: 'x', admin: true } as Record<string, unknown>)).toMatchObject({
      url: 'x',
      email: '',
      owner: false,
    })
    expect(fieldsFromForm('url=x&url=y').url).toBe('y')
  })
})

describe('validateSubmission', () => {
  it('accepts a good submission and normalises the URL', async () => {
    const result = await validate({ url: 'example.org/pricing#top', intent: '  Find  the pricing page ' })
    expect(result.ok).toBe(true)
    if (!result.ok) return
    expect(result.value.url).toBe('https://example.org/pricing')
    expect(result.value.host).toBe('example.org')
    expect(result.value.intent).toBe('Find the pricing page')
    expect(result.value.email).toBe(EMAIL)
  })

  it('rejects a non-https URL', async () => {
    const result = await validate({ url: 'http://example.org' })
    expect(result.ok).toBe(false)
    if (result.ok) return
    expect(result.error.error.code).toBe('invalid_url')
    expect(result.error.error.status).toBe(400)
    expect(result.error.ok).toBe(false)
  })

  it('rejects loopback and IP-literal hosts without touching DNS', async () => {
    const lookup = vi.fn(publicLookup)
    for (const url of ['https://localhost/x', 'https://127.0.0.1/', 'https://192.168.1.10/', 'https://api.internal/']) {
      const result = await validateSubmission(fields({ url }), { lookup })
      expect(result.ok, url).toBe(false)
    }
    expect(lookup).not.toHaveBeenCalled()
  })

  it('rejects a public-looking host that resolves to a private address', async () => {
    const result = await validate({ url: 'https://intranet.example.org/' }, privateLookup)
    expect(result.ok).toBe(false)
    if (result.ok) return
    expect(result.error.error.code).toBe('invalid_host')
    expect(result.error.error.message).toContain('10.0.0.5')
  })

  it('rejects a host that does not resolve', async () => {
    const result = await validate({ url: 'https://nope.example.org/' }, async () => [])
    expect(result.ok).toBe(false)
    if (result.ok) return
    expect(result.error.error.code).toBe('invalid_host')
  })

  it('rejects a malformed or oversize email', async () => {
    for (const email of ['', 'chip', 'chip@', '@example.com', 'chip@example', 'a b@example.com', 'chip@ex ample.com']) {
      const result = await validate({ email })
      expect(result.ok, email).toBe(false)
      if (result.ok) return
      expect(result.error.error.code).toBe('invalid_email')
    }

    const long = `${'a'.repeat(250)}@example.com`
    const result = await validate({ email: long })
    expect(result.ok).toBe(false)
    if (result.ok) return
    expect(result.error.error.message).toContain('254')
  })

  it('rejects a filled honeypot without saying why', async () => {
    const result = await validate({ website: 'https://spam.example' })
    expect(result.ok).toBe(false)
    if (result.ok) return
    expect(result.error.error.code).toBe('rejected')
    expect(result.error.error.message).toBe('Submission rejected.')
  })

  it('rejects an oversize intent', async () => {
    const result = await validate({ intent: 'x'.repeat(MAX_INTENT_LENGTH + 1) })
    expect(result.ok).toBe(false)
    if (result.ok) return
    expect(result.error.error.code).toBe('intent_too_long')
  })

  it('strips control characters from the intent before it is stored', () => {
    expect(sanitizeIntent('Book\0 a\n table\x7f')).toBe('Book a table')
  })

  it('strips shell and quoting metacharacters from the intent', () => {
    // The intent is prose. None of these are needed to say what an agent
    // should do, and all of them are load-bearing in a shell or a prompt.
    expect(sanitizeIntent('Book a table $(id) `whoami`')).toBe('Book a table id whoami')
    expect(sanitizeIntent(`Find "the" 'pricing' page`)).toBe('Find the pricing page')
    expect(sanitizeIntent('a; rm -rf / && echo b | c')).toBe('a rm -rf / echo b c')
    expect(sanitizeIntent('a\\b <c> {d} !e')).toBe('a b c d e')
    for (const ch of ['$', '`', '\\', '"', "'", '!', '(', ')', '|', ';', '&', '<', '>', '{', '}']) {
      expect(sanitizeIntent(`x${ch}y`), ch).toBe('x y')
    }
  })

  it('accepts ordinary addresses and rejects an oversize local part', () => {
    expect(isValidEmail('chip.lay+atlas@sub.example.co.uk')).toBe(true)
    expect(isValidEmail(`${'a'.repeat(65)}@example.com`)).toBe(false)
  })
})

// ---------------------------------------------------------------------------

const VALUE: ValidSubmission = {
  url: 'https://example.org/pricing',
  host: 'example.org',
  email: EMAIL,
  emailNormalized: EMAIL,
  owner: true,
  sightkick: true,
  intent: 'Find the pricing page',
  nominate: false,
  rescan: false,
}

function record() {
  return buildRecord(VALUE, {
    id: 'abc123',
    receivedAt: '2026-09-10T12:00:00.000Z',
    emailHash: 'deadbeef',
    ipHash: 'cafef00d',
    userAgent: 'Mozilla/5.0',
    runner: { kind: 'netlify' },
  })
}

describe('buildRecord', () => {
  it('keeps every submitted field, plus the hashes and state', () => {
    expect(record()).toEqual({
      id: 'abc123',
      receivedAt: '2026-09-10T12:00:00.000Z',
      url: 'https://example.org/pricing',
      host: 'example.org',
      email: EMAIL,
      emailHash: 'deadbeef',
      owner: true,
      sightkick: true,
      intent: 'Find the pricing page',
      nominate: false,
      rescan: false,
      ipHash: 'cafef00d',
      userAgent: 'Mozilla/5.0',
      state: 'new',
      runner: { kind: 'netlify' },
    })
  })

  it('files the record by day and points an index blob at it', () => {
    expect(recordKey('2026-09-10T12:00:00.000Z', 'abc123')).toBe('2026-09-10/abc123')
    expect(indexKey('abc123')).toBe('index/abc123')
  })

  it('truncates a runaway user agent', () => {
    const long = buildRecord(VALUE, {
      id: 'a',
      receivedAt: '2026-09-10T12:00:00.000Z',
      emailHash: 'h',
      ipHash: 'i',
      userAgent: 'x'.repeat(1000),
      runner: { kind: 'queue' },
    })
    expect(long.userAgent).toHaveLength(300)
  })

  it('mints a short URL-safe id', () => {
    expect(newSubmissionId('0f4a2c1e-1111-2222-3333-444455556666')).toBe('0f4a2c1e11112222')
    expect(newSubmissionId()).toMatch(/^[0-9a-f]{16}$/)
  })
})

describe('publicStatus', () => {
  it('exposes state and host but never the email or the full URL', () => {
    const status = publicStatus({ ...record(), runner: { kind: 'netlify', id: 'run_1', state: 'running' } })
    expect(status).toEqual({
      ok: true,
      id: 'abc123',
      state: 'new',
      receivedAt: '2026-09-10T12:00:00.000Z',
      host: 'example.org',
      runner: { kind: 'netlify', state: 'running' },
    })
    const serialised = JSON.stringify(status)
    expect(serialised).not.toContain(EMAIL)
    expect(serialised).not.toContain('/pricing')
    expect(serialised).not.toContain('run_1')
  })
})

describe('rateDecision', () => {
  const now = 1_000_000

  it('opens a window on the first submission', () => {
    const decision = rateDecision(null, now)
    expect(decision.allowed).toBe(true)
    expect(decision.state.count).toBe(1)
    expect(decision.state.resetAt).toBe(now + 24 * 60 * 60 * 1000)
  })

  it('counts up to five and then blocks with a Retry-After', () => {
    let state = rateDecision(null, now).state
    for (let i = 2; i <= 5; i += 1) {
      const decision = rateDecision(state, now)
      expect(decision.allowed).toBe(true)
      expect(decision.state.count).toBe(i)
      state = decision.state
    }
    const blocked = rateDecision(state, now)
    expect(blocked.allowed).toBe(false)
    expect(blocked.retryAfterSeconds).toBe(24 * 60 * 60)
    expect(rateLimitedError(blocked.retryAfterSeconds).error.status).toBe(429)
  })

  it('starts a fresh window once the old one has expired', () => {
    const expired = { count: 5, resetAt: now - 1 }
    expect(rateDecision(expired, now)).toMatchObject({ allowed: true, state: { count: 1 } })
  })

  it('treats unreadable stored state as no history', () => {
    expect(parseRateState(null)).toBeNull()
    expect(parseRateState('garbage')).toBeNull()
    expect(parseRateState({ count: 'x', resetAt: 1 })).toBeNull()
    expect(parseRateState({ count: 2, resetAt: 5 })).toEqual({ count: 2, resetAt: 5 })
  })
})

describe('hashing', () => {
  it('salts the IP hash per deploy and never reveals the address', async () => {
    const a = await hashIp('203.0.113.7', 'atlas')
    const b = await hashIp('203.0.113.7', 'other-deploy')
    expect(a).toMatch(/^[0-9a-f]{64}$/)
    expect(a).not.toBe(b)
    expect(await hashIp('203.0.113.7', 'atlas')).toBe(a)
  })

  it('hashes the email case-insensitively so one mailbox is one hash', async () => {
    expect(await hashEmail('Chip@Example.com', 'atlas')).toBe(await hashEmail('chip@example.com', 'atlas'))
    expect(await hashEmail(EMAIL, 'atlas')).not.toContain('example')
    expect(await hashEmail(EMAIL, 'atlas')).toMatch(/^[0-9a-f]{64}$/)
  })

  it('keys the email hash so a known address cannot be confirmed by digesting it', async () => {
    // A plain SHA-256 of the address is one guess away from being reversed.
    expect(await hashEmail(EMAIL, 'atlas')).not.toBe(await sha256Hex(EMAIL))
    // The key is what makes two hashes comparable — changing ATLAS_SUBMIT_SALT
    // retires every hash already on file.
    expect(await hashEmail(EMAIL, 'atlas')).not.toBe(await hashEmail(EMAIL, 'other-deploy'))
    expect(await hmacSha256Hex('atlas', EMAIL)).toBe(await hashEmail(EMAIL, 'atlas'))
  })
})

// ---------------------------------------------------------------------------

const TOKEN = '0123456789abcdef0123456789abcdef'

describe('claim', () => {
  it('reads the token from either encoding, lowercased', () => {
    expect(readFields({ claim: TOKEN.toUpperCase() }).claim).toBe(TOKEN)
    expect(fieldsFromForm(`url=https%3A%2F%2Fa.example&claim=${TOKEN}`).claim).toBe(TOKEN)
    expect(readFields({}).claim).toBe('')
  })

  it('accepts a well-formed token and carries it no further than the request', async () => {
    const result = await validate({ claim: TOKEN })
    expect(result.ok).toBe(true)
    if (!result.ok) return
    expect(result.value.claim).toBe(TOKEN)

    // The record keeps the date, never the token.
    const record = buildRecord(result.value, {
      id: 'abc',
      receivedAt: '2026-09-10T00:00:00.000Z',
      emailHash: 'hash',
      ipHash: 'ip',
      userAgent: '',
      runner: { kind: 'queue' },
      claim: { verifiedAt: '2026-09-10T00:00:00.000Z' },
    })
    expect(record.claim).toEqual({ verifiedAt: '2026-09-10T00:00:00.000Z' })
    expect(JSON.stringify(record)).not.toContain(TOKEN)
  })

  it('rejects anything that is not 32 lowercase hex characters', async () => {
    for (const claim of ['not-a-token', TOKEN.slice(1), `${TOKEN}0`, 'zzzz']) {
      const result = await validate({ claim })
      expect(result.ok).toBe(false)
      if (result.ok) return
      expect(result.error.error).toMatchObject({ code: 'claim-invalid', status: 400 })
    }
  })

  it('leaves a submission without a claim exactly as it was', async () => {
    const result = await validate()
    expect(result.ok).toBe(true)
    if (!result.ok) return
    expect(result.value.claim).toBe('')
    expect(buildRecord(result.value, {
      id: 'abc',
      receivedAt: '2026-09-10T00:00:00.000Z',
      emailHash: 'hash',
      ipHash: 'ip',
      userAgent: '',
      runner: { kind: 'queue' },
    })).not.toHaveProperty('claim')
    expect(acceptedBody('abc')).not.toHaveProperty('card')
  })
})

describe('buildTryRecord', () => {
  const value: ValidSubmission = {
    url: 'https://www.example.org/pricing',
    host: 'www.example.org',
    email: EMAIL,
    emailNormalized: EMAIL,
    owner: true,
    sightkick: false,
    intent: '',
    nominate: false,
    rescan: false,
    claim: TOKEN,
  }

  it('keys the card on the canonical host and starts a 30-day life', () => {
    const record = buildTryRecord(value, { id: 'abc', claimedAt: '2026-09-10T00:00:00.000Z' })
    expect(record).toEqual({
      v: 1,
      // The same string /atlas/hosts/<host>.json is keyed by, so the card can
      // tell "listed" from "unlisted" with one lookup.
      host: 'example.org',
      url: 'https://www.example.org/pricing',
      submissionId: 'abc',
      claimedAt: '2026-09-10T00:00:00.000Z',
      expiresAt: '2026-10-10T00:00:00.000Z',
    })
  })

  it('carries nothing about the submitter', () => {
    const record = buildTryRecord(value, { id: 'abc', claimedAt: '2026-09-10T00:00:00.000Z' })
    const serialised = JSON.stringify(record)
    expect(serialised).not.toContain(EMAIL)
    expect(serialised).not.toContain(TOKEN)
  })

  it('points the card at the same host', () => {
    expect(cardUrl('www.example.org')).toBe('https://sightmap.org/try/example.org')
    expect(acceptedBody('abc', cardUrl('example.org')).card).toBe('https://sightmap.org/try/example.org')
  })
})

describe('claim and quarantine errors', () => {
  it('answers a failed claim 422, in the shared error shape', () => {
    const body = claimRejectedError('claim-mismatch')
    expect(body.ok).toBe(false)
    expect(body.error).toMatchObject({ code: 'claim-mismatch', status: 422 })
    expect(body.error.hint).toContain('sightmap-claim')

    expect(claimRejectedError('claim-unreachable').error).toMatchObject({
      code: 'claim-unreachable',
      status: 422,
    })
  })

  it('never says why the host could not be read: the check is not a probe', () => {
    const message = claimRejectedError('claim-unreachable').error.message
    expect(message).not.toMatch(/HTTP|\d{3}|timed out|redirect/)
  })

  it('answers a quarantined host 403 without saying why', () => {
    const body = quarantinedError('example.org')
    expect(body.error).toMatchObject({ code: 'quarantined', status: 403 })
    expect(body.error.message).toContain('example.org')
  })
})
