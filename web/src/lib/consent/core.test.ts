import { describe, expect, it } from 'vitest'
import {
  CONSENT_MAX_AGE_MS,
  parseStoredConsent,
  readConsentCookieValue,
  resolveConsentDecision,
  serializeConsentCookie,
} from './core'

describe('resolveConsentDecision', () => {
  it('REQUIRED + no choice → block + auto-show', () => {
    expect(resolveConsentDecision('REQUIRED', null)).toEqual({
      loadCapture: false,
      autoShowBanner: true,
    })
  })
  it('OPTIONAL + no choice → capture on, no banner', () => {
    expect(resolveConsentDecision('OPTIONAL', null)).toEqual({
      loadCapture: true,
      autoShowBanner: false,
    })
  })
  it('stored choice always wins and never auto-shows', () => {
    const g = { status: 'granted', v: 1, ts: 0 } as const
    const d = { status: 'denied', v: 1, ts: 0 } as const
    expect(resolveConsentDecision('REQUIRED', g)).toEqual({
      loadCapture: true,
      autoShowBanner: false,
    })
    expect(resolveConsentDecision('REQUIRED', d)).toEqual({
      loadCapture: false,
      autoShowBanner: false,
    })
    expect(resolveConsentDecision('OPTIONAL', g)).toEqual({
      loadCapture: true,
      autoShowBanner: false,
    })
    expect(resolveConsentDecision('OPTIONAL', d)).toEqual({
      loadCapture: false,
      autoShowBanner: false,
    })
  })
})

describe('parseStoredConsent', () => {
  const now = 1_000_000_000_000
  it('parses a valid cookie value', () => {
    expect(parseStoredConsent(JSON.stringify({ status: 'granted', v: 1, ts: now }), now)).toEqual({
      status: 'granted',
      v: 1,
      ts: now,
    })
  })
  it('returns null for missing/malformed/wrong-version/bad-status', () => {
    expect(parseStoredConsent(null, now)).toBeNull()
    expect(parseStoredConsent('not json', now)).toBeNull()
    expect(parseStoredConsent(JSON.stringify({ status: 'granted', v: 2, ts: now }), now)).toBeNull()
    expect(parseStoredConsent(JSON.stringify({ status: 'maybe', v: 1, ts: now }), now)).toBeNull()
  })
  it('returns null once older than the max age', () => {
    const old = now - CONSENT_MAX_AGE_MS - 1
    expect(parseStoredConsent(JSON.stringify({ status: 'granted', v: 1, ts: old }), now)).toBeNull()
  })
})

describe('cookie string helpers', () => {
  it('extracts the consent value among other cookies', () => {
    const raw = 'theme=dark; sightmap_consent=%7B%22a%22%3A1%7D; other=x'
    expect(readConsentCookieValue(raw)).toBe('{"a":1}')
  })
  it('returns null when absent or empty', () => {
    expect(readConsentCookieValue('theme=dark')).toBeNull()
    expect(readConsentCookieValue('')).toBeNull()
  })
  it('serializes a cookie that round-trips through parse', () => {
    const now = 1_700_000_000_000
    const cookie = serializeConsentCookie('denied', now)
    expect(cookie).toContain('sightmap_consent=')
    expect(cookie).toContain('Max-Age=15552000')
    expect(cookie).toContain('Path=/')
    expect(cookie).toContain('SameSite=Lax')
    expect(cookie).toContain('Secure')
    const value = readConsentCookieValue(cookie.split(';')[0])
    expect(parseStoredConsent(value, now)).toEqual({ status: 'denied', v: 1, ts: now })
  })
})
