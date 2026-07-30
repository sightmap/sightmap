// Browser glue: read/write the consent cookie and start/stop capture to match.
import {
  parseStoredConsent,
  readConsentCookieValue,
  serializeConsentCookie,
  type ConsentRegion,
  type ConsentStatus,
  type StoredConsent,
} from './core'

declare global {
  interface Window {
    /** Fullstory loader, deferred behind consent (defined in index.html). */
    __sightmapStartCapture?: () => void
    /** Consent region injected by the geo-region edge function. */
    __CC_REGION__?: ConsentRegion
    /** Snapshot the inline gate leaves for React to read on mount. */
    __CC_INITIAL?: { region: ConsentRegion; stored: StoredConsent | null }
    /** Fullstory SDK, present only once the loader has run. */
    FS?: (command: 'restart' | 'shutdown', ...args: unknown[]) => void
  }
}

export function readConsent(now: number = Date.now()): StoredConsent | null {
  if (typeof document === 'undefined') return null
  return parseStoredConsent(readConsentCookieValue(document.cookie), now)
}

export function writeConsent(status: ConsentStatus): void {
  if (typeof document === 'undefined') return
  document.cookie = serializeConsentCookie(status, Date.now())
}

/** Start or stop Fullstory capture to match the choice. Idempotent. */
export function applyConsent(status: ConsentStatus): void {
  if (typeof window === 'undefined') return
  if (status === 'granted') {
    if (typeof window.FS === 'function') {
      window.FS('restart')
    } else {
      window.__sightmapStartCapture?.()
    }
  } else if (typeof window.FS === 'function') {
    window.FS('shutdown')
  }
}
