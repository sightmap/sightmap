/* eslint-disable react-refresh/only-export-components */
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react'
import {
  resolveConsentDecision,
  type ConsentRegion,
  type ConsentStatus,
  type StoredConsent,
} from '@/lib/consent/core'
import { applyConsent, readConsent, writeConsent } from '@/lib/consent'

interface ConsentContextValue {
  region: ConsentRegion
  status: ConsentStatus | null
  captureEnabled: boolean
  showBanner: boolean
  isPreferencesOpen: boolean
  openPreferences: () => void
  closePreferences: () => void
  setConsent: (status: ConsentStatus) => void
}

const ConsentContext = createContext<ConsentContextValue | null>(null)

interface Initial {
  region: ConsentRegion
  stored: StoredConsent | null
}

function readInitial(): Initial {
  if (typeof window === 'undefined') return { region: 'REQUIRED', stored: null }
  if (window.__CC_INITIAL) return window.__CC_INITIAL
  return { region: window.__CC_REGION__ || 'REQUIRED', stored: readConsent() }
}

export function ConsentProvider({ children }: { children: ReactNode }) {
  // The site is prerendered, so first render also happens in Node with no
  // window. Nothing consent-related is shown until `mounted` flips on the
  // client, which keeps the static HTML free of a baked-in banner.
  const [initial, setInitial] = useState<Initial>({ region: 'REQUIRED', stored: null })
  const [mounted, setMounted] = useState(false)
  const [dismissed, setDismissed] = useState(false)
  const [isPreferencesOpen, setPreferencesOpen] = useState(false)

  useEffect(() => {
    setInitial(readInitial())
    setMounted(true)
  }, [])

  const region = initial.region
  const decision = resolveConsentDecision(region, initial.stored)

  const setConsent = useCallback((next: ConsentStatus) => {
    writeConsent(next)
    applyConsent(next)
    setInitial((prev) => ({ ...prev, stored: readConsent() }))
    setDismissed(true)
    setPreferencesOpen(false)
  }, [])

  const openPreferences = useCallback(() => setPreferencesOpen(true), [])
  const closePreferences = useCallback(() => setPreferencesOpen(false), [])

  const value = useMemo<ConsentContextValue>(
    () => ({
      region,
      status: initial.stored?.status ?? null,
      captureEnabled: decision.loadCapture,
      showBanner: mounted && decision.autoShowBanner && !dismissed,
      isPreferencesOpen: mounted && isPreferencesOpen,
      openPreferences,
      closePreferences,
      setConsent,
    }),
    [
      region,
      initial.stored,
      decision.loadCapture,
      decision.autoShowBanner,
      mounted,
      dismissed,
      isPreferencesOpen,
      openPreferences,
      closePreferences,
      setConsent,
    ]
  )

  return <ConsentContext.Provider value={value}>{children}</ConsentContext.Provider>
}

export function useConsent(): ConsentContextValue {
  const ctx = useContext(ConsentContext)
  if (!ctx) throw new Error('useConsent must be used within ConsentProvider')
  return ctx
}
