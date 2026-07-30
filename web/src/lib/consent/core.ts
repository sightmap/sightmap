// Pure consent logic — no DOM, no React. Single source of truth for the
// decision table and cookie encoding. Mirrored (loadCapture rule only) by the
// inline gate in index.html, which must run before this bundle loads.

export type ConsentStatus = 'granted' | 'denied'
export type ConsentRegion = 'REQUIRED' | 'OPTIONAL'

export interface StoredConsent {
  status: ConsentStatus
  v: number
  ts: number
}

export interface ConsentDecision {
  loadCapture: boolean
  autoShowBanner: boolean
}

export const CONSENT_COOKIE_NAME = 'sightmap_consent'
export const POLICY_VERSION = 1
export const CONSENT_MAX_AGE_MS = 180 * 24 * 60 * 60 * 1000

/** Given region and any stored choice, decide capture + whether to auto-show the banner. */
export function resolveConsentDecision(
  region: ConsentRegion,
  stored: StoredConsent | null
): ConsentDecision {
  if (stored) {
    return { loadCapture: stored.status === 'granted', autoShowBanner: false }
  }
  if (region === 'OPTIONAL') {
    return { loadCapture: true, autoShowBanner: false }
  }
  return { loadCapture: false, autoShowBanner: true }
}

/** Parse a raw cookie value into a valid, unexpired, current-version choice, or null. */
export function parseStoredConsent(raw: string | null, now: number): StoredConsent | null {
  if (!raw) return null
  let parsed: unknown
  try {
    parsed = JSON.parse(raw)
  } catch {
    return null
  }
  if (typeof parsed !== 'object' || parsed === null) return null
  const o = parsed as Record<string, unknown>
  if (o.status !== 'granted' && o.status !== 'denied') return null
  if (o.v !== POLICY_VERSION) return null
  if (typeof o.ts !== 'number') return null
  if (now - o.ts > CONSENT_MAX_AGE_MS) return null
  return { status: o.status, v: o.v, ts: o.ts }
}

/** Extract the decoded `sightmap_consent` value from a `document.cookie` string. */
export function readConsentCookieValue(cookieString: string): string | null {
  const parts = cookieString ? cookieString.split('; ') : []
  for (const part of parts) {
    const eq = part.indexOf('=')
    if (eq === -1) continue
    if (part.slice(0, eq) === CONSENT_COOKIE_NAME) {
      return decodeURIComponent(part.slice(eq + 1))
    }
  }
  return null
}

/** Build the `document.cookie` assignment string for a choice. */
export function serializeConsentCookie(status: ConsentStatus, now: number): string {
  const value: StoredConsent = { status, v: POLICY_VERSION, ts: now }
  const encoded = encodeURIComponent(JSON.stringify(value))
  const maxAgeSec = Math.floor(CONSENT_MAX_AGE_MS / 1000)
  return `${CONSENT_COOKIE_NAME}=${encoded}; Max-Age=${maxAgeSec}; Path=/; SameSite=Lax; Secure`
}
