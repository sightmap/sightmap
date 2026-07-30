import { useEffect, useRef } from 'react'
import { useConsent } from './ConsentContext'

const PRIVACY_URL = 'https://www.fullstory.com/legal/privacy-policy/'

export default function ConsentUI() {
  const { showBanner, isPreferencesOpen } = useConsent()
  if (isPreferencesOpen) return <PreferencesPanel />
  if (showBanner) return <ConsentBanner />
  return null
}

function ConsentBanner() {
  const { setConsent } = useConsent()
  return (
    <div
      data-component="ConsentBanner"
      className="consent-banner"
      role="region"
      aria-label="Cookie consent"
    >
      <p className="consent-copy">
        We use cookies to capture how the site is used and improve it. See our{' '}
        <a href={PRIVACY_URL} data-fs-element="Sightmap | Consent: Privacy Policy">
          Privacy Policy
        </a>
        .
      </p>
      <div className="consent-actions">
        <button
          type="button"
          onClick={() => setConsent('granted')}
          data-action="consent-accept"
          data-fs-element="Sightmap | Consent: Accept"
          className="consent-btn consent-btn-primary"
        >
          Accept
        </button>
        <button
          type="button"
          onClick={() => setConsent('denied')}
          data-action="consent-decline"
          data-fs-element="Sightmap | Consent: Decline"
          className="consent-btn"
        >
          Decline
        </button>
      </div>
    </div>
  )
}

function PreferencesPanel() {
  const { status, captureEnabled, closePreferences, setConsent } = useConsent()
  const dialogRef = useRef<HTMLDivElement>(null)

  // Default the toggle to current capture state (on unless explicitly denied).
  const initialOn = status ? status === 'granted' : captureEnabled
  const toggleRef = useRef(initialOn)

  useEffect(() => {
    const dialog = dialogRef.current
    const previouslyFocused = document.activeElement as HTMLElement | null
    const getFocusable = () =>
      dialog
        ? Array.from(
            dialog.querySelectorAll<HTMLElement>(
              'a[href], button:not([disabled]), input:not([disabled]), [tabindex]:not([tabindex="-1"])'
            )
          )
        : []

    // Focus the first control on open so the dialog owns focus immediately.
    ;(getFocusable()[0] ?? dialog)?.focus()

    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        closePreferences()
        return
      }
      if (e.key !== 'Tab') return
      // Trap Tab within the dialog so focus cannot land on the page behind it.
      const items = getFocusable()
      if (items.length === 0) {
        e.preventDefault()
        dialog?.focus()
        return
      }
      const first = items[0]
      const last = items[items.length - 1]
      const active = document.activeElement
      if (e.shiftKey && (active === first || active === dialog)) {
        e.preventDefault()
        last.focus()
      } else if (!e.shiftKey && active === last) {
        e.preventDefault()
        first.focus()
      }
    }

    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('keydown', onKey)
      // Restore focus to whatever opened the dialog (e.g. the footer link).
      previouslyFocused?.focus?.()
    }
  }, [closePreferences])

  return (
    <div className="consent-overlay" onClick={closePreferences}>
      <div
        ref={dialogRef}
        data-component="ConsentPanel"
        className="consent-panel"
        role="dialog"
        aria-modal="true"
        aria-label="Privacy preferences"
        tabIndex={-1}
        onClick={(e) => e.stopPropagation()}
      >
        <h2 className="consent-title">Privacy preferences</h2>
        <p className="consent-copy">
          Manage optional capture. Necessary cookies stay on so the site works.
        </p>

        <div className="consent-rows">
          <div className="consent-row">
            <div>
              <div className="consent-row-name">Necessary</div>
              <div className="consent-row-desc">Consent preferences.</div>
            </div>
            <span className="consent-row-locked">Always on</span>
          </div>

          <label className="consent-row consent-row-toggle">
            <div>
              <div className="consent-row-name">Analytics: session capture</div>
              <div className="consent-row-desc">
                Fullstory session capture, used to understand and improve the site.
              </div>
            </div>
            <input
              type="checkbox"
              defaultChecked={initialOn}
              onChange={(e) => {
                toggleRef.current = e.target.checked
              }}
              data-action="consent-toggle-analytics"
              data-fs-element="Sightmap | Consent: Analytics Toggle"
            />
          </label>
        </div>

        <div className="consent-actions consent-actions-end">
          <button
            type="button"
            onClick={closePreferences}
            data-action="consent-cancel"
            data-fs-element="Sightmap | Consent: Cancel"
            className="consent-btn"
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={() => setConsent(toggleRef.current ? 'granted' : 'denied')}
            data-action="consent-save"
            data-fs-element="Sightmap | Consent: Save"
            className="consent-btn consent-btn-primary"
          >
            Save Preferences
          </button>
        </div>
      </div>
    </div>
  )
}
