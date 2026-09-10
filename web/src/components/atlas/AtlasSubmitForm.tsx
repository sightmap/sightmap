import { useEffect, useState } from 'react'
import { CLAIM_TOKEN_LENGTH, CLAIM_TOKEN_PATTERN, SUBMIT_ENDPOINT } from '@/lib/submit-types'
import type { SubmitRequest, SubmitResponse } from '@/lib/submit-types'

interface Props {
  /** From `?url=` — a listing's "Request a rescan" link lands here prefilled. */
  initialUrl?: string
  /** From `?rescan=1`. Carried through as a hidden field, never shown. */
  rescan?: boolean
  /** From `?submitted=<id>` — the no-JS redirect target. */
  submittedId?: string
  /** From `?card=<url>` — the card a verified claim earned, on the no-JS path. */
  cardUrl?: string
  /** From `?error=<code>` — where a no-JS post that failed validation lands. */
  errorCode?: string
}

type State =
  | { name: 'idle' }
  | { name: 'sending' }
  | { name: 'received'; id?: string; card?: string }
  | { name: 'error'; message: string }

const RECEIVED_MESSAGE =
  'We normally scan submissions within one business day; the report goes to your email.'

/**
 * The submit form for a free scan.
 *
 * Progressive enhancement, not a JS-only widget: this is a real
 * `<form method="post" action={SUBMIT_ENDPOINT}>` that posts urlencoded fields
 * and works with scripting off, and the submit handler upgrades it to a JSON
 * fetch when React is running so the visitor stays on the page. The no-JS path
 * comes back as a redirect to `/atlas?submitted=<id>#submit`, which the page
 * reads and hands back here as `submittedId` — so both paths end on the same
 * confirmation copy rather than two different-looking successes.
 *
 * The `website` input is a honeypot: it is off-screen and out of the tab order,
 * so a person never fills it and a form-filling bot fills everything.
 */
// The no-JS path cannot carry the function's JSON error, only its code.
const ERROR_MESSAGES: Record<string, string> = {
  invalid_url: 'That URL could not be accepted. Use a public https:// address without credentials.',
  invalid_host: 'That hostname is not public, so it cannot be scanned.',
  invalid_email: 'That email address does not look valid.',
  intent_too_long: 'The intent is too long; keep it under 500 characters.',
  rate_limited: 'Too many submissions from this network today. Try again tomorrow.',
  'claim-invalid': 'A claim token is 32 hex characters. Leave the field empty if you do not have one.',
  'claim-unreachable':
    'We could not read https://<your host>/webmcp.txt. Publish it, then submit again — nothing was stored.',
  'claim-mismatch':
    'The claim line in your webmcp.txt does not match that token. Fix either one and submit again — nothing was stored.',
  quarantined: 'Submissions for that site are not being accepted. Email hello@sightmap.org if that looks wrong.',
}
const errorMessage = (code: string): string =>
  ERROR_MESSAGES[code] ?? 'The submission could not be recorded. Check the fields and try again.'

export default function AtlasSubmitForm({ initialUrl = '', rescan = false, submittedId, cardUrl, errorCode }: Props) {
  const [url, setUrl] = useState('')
  const [email, setEmail] = useState('')
  const [owner, setOwner] = useState(false)
  const [sightkick, setSightkick] = useState(false)
  const [nominate, setNominate] = useState(false)
  const [intent, setIntent] = useState('')
  const [claim, setClaim] = useState('')
  const [state, setState] = useState<State>({ name: 'idle' })

  // Both of these arrive one commit after mount, because the page only reads
  // the query string after hydration (one static file serves every query
  // string for this route — see the note in AtlasIndex).
  useEffect(() => {
    if (initialUrl) setUrl(initialUrl)
  }, [initialUrl])

  useEffect(() => {
    if (submittedId) setState({ name: 'received', id: submittedId, card: cardUrl })
    else if (errorCode) setState({ name: 'error', message: errorMessage(errorCode) })
  }, [submittedId, cardUrl, errorCode])

  const submit = async (event: React.FormEvent<HTMLFormElement>) => {
    // Only intercept once we know we can do better than the native post.
    event.preventDefault()
    const form = event.currentTarget
    const honeypot = new FormData(form).get('website')
    setState({ name: 'sending' })

    const payload: SubmitRequest = {
      url,
      email,
      owner,
      sightkick,
      nominate,
      intent,
      rescan,
      claim,
      website: typeof honeypot === 'string' ? honeypot : '',
    }

    try {
      const res = await fetch(SUBMIT_ENDPOINT, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      })
      // A function that fell over returns HTML, not JSON, so the parse is
      // guarded too — otherwise a 502 surfaces as "Unexpected token <".
      const data = (await res.json().catch(() => null)) as SubmitResponse | null
      if (!res.ok || !data || !data.ok) {
        setState({
          name: 'error',
          message: data?.error?.message ?? `The submission could not be recorded (HTTP ${res.status}).`,
        })
        return
      }
      setState({ name: 'received', id: data.id, card: data.card })
    } catch {
      setState({ name: 'error', message: 'The submission could not be sent. Check your connection and try again.' })
    }
  }

  if (state.name === 'received') {
    return (
      <section className="atlas-submit-form" id="submit" data-component="AtlasSubmitForm" aria-labelledby="atlas-submit-h">
        <h2 id="atlas-submit-h" className="atlas-listing__h2">
          Submission received
        </h2>
        <div className="atlas-submit-form__received" role="status">
          {state.id && (
            <p className="atlas-submit-form__id">
              Reference <code>{state.id}</code>
            </p>
          )}
          <p>{RECEIVED_MESSAGE}</p>
          {state.card && (
            <p className="atlas-submit-form__card">
              Your card is at{' '}
              <a href={state.card} data-component="SubmitCardLink">
                {state.card}
              </a>
              . It is unlisted and not indexed; the Atlas listing itself follows a maintainer review.
            </p>
          )}
        </div>
      </section>
    )
  }

  return (
    <section className="atlas-submit-form" id="submit" data-component="AtlasSubmitForm" aria-labelledby="atlas-submit-h">
      <h2 id="atlas-submit-h" className="atlas-listing__h2">
        Submit a site for a scan
      </h2>
      <p className="atlas-submit-form__intro">
        The scan enumerates the WebMCP tools a page registers and never calls one. You get the
        report, and a Sightkick starter either way — a replayable check if tools were found, a
        drafted tool layer if none were.
      </p>

      <form
        className="atlas-submit-form__form"
        method="post"
        action={SUBMIT_ENDPOINT}
        onSubmit={submit}
        data-component="AtlasSubmitFields"
      >
        <label className="atlas-submit-form__field">
          <span className="atlas-submit-form__label">Site URL</span>
          <input
            type="url"
            name="url"
            required
            placeholder="https://example.com/"
            value={url}
            onChange={(e) => setUrl(e.target.value)}
            data-component="SubmitUrl"
          />
        </label>

        <label className="atlas-submit-form__field">
          <span className="atlas-submit-form__label">Email</span>
          <input
            type="email"
            name="email"
            required
            placeholder="you@example.com"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            data-component="SubmitEmail"
          />
          <span className="atlas-submit-form__hint">Where the report goes. Not published.</span>
        </label>

        <div className="atlas-submit-form__checks">
          <label className="atlas-submit-form__check">
            <input
              type="checkbox"
              name="owner"
              checked={owner}
              onChange={(e) => setOwner(e.target.checked)}
              data-component="SubmitOwner"
            />
            <span>I own or maintain this site</span>
          </label>
          <label className="atlas-submit-form__check">
            <input
              type="checkbox"
              name="sightkick"
              checked={sightkick}
              onChange={(e) => setSightkick(e.target.checked)}
              data-component="SubmitSightkick"
            />
            <span>This site uses Sightkick</span>
          </label>
          <label className="atlas-submit-form__check">
            <input
              type="checkbox"
              name="nominate"
              checked={nominate}
              onChange={(e) => setNominate(e.target.checked)}
              data-component="SubmitNominate"
            />
            <span>I&rsquo;m nominating someone else&rsquo;s site</span>
          </label>
        </div>

        <label className="atlas-submit-form__field">
          <span className="atlas-submit-form__label">What should an agent be able to do here?</span>
          <textarea
            name="intent"
            rows={3}
            maxLength={500}
            placeholder="Find a flight and hold a seat."
            value={intent}
            onChange={(e) => setIntent(e.target.value)}
            data-component="SubmitIntent"
          />
          <span className="atlas-submit-form__hint">
            Optional, 500 characters. Stored with the scan and read by the review agent; nothing is
            executed on your site.
          </span>
        </label>

        <label className="atlas-submit-form__field">
          <span className="atlas-submit-form__label">Claim token</span>
          <input
            type="text"
            name="claim"
            autoComplete="off"
            spellCheck={false}
            pattern={CLAIM_TOKEN_PATTERN}
            maxLength={CLAIM_TOKEN_LENGTH}
            placeholder="0123456789abcdef0123456789abcdef"
            value={claim}
            onChange={(e) => setClaim(e.target.value.trim().toLowerCase())}
            data-component="SubmitClaim"
          />
          <span className="atlas-submit-form__hint">
            The token in your <code>webmcp.txt</code>; optional, gives you a shareable card right away.
          </span>
        </label>

        {/* Carried so the no-JS post says the same thing the fetch payload does. */}
        <input type="hidden" name="rescan" value={rescan ? '1' : ''} />

        {/* Honeypot. Hidden in CSS, out of the tab order, never announced. */}
        <div className="atlas-submit-form__hp" aria-hidden="true">
          <label>
            Website
            <input type="text" name="website" tabIndex={-1} autoComplete="off" defaultValue="" />
          </label>
        </div>

        <div className="atlas-submit-form__actions">
          <button
            type="submit"
            className="btn-primary"
            disabled={state.name === 'sending'}
            data-component="SubmitButton"
          >
            {state.name === 'sending' ? 'Sending…' : 'Scan my site'}
          </button>
          {state.name === 'error' && (
            <p className="atlas-submit-form__error" role="alert">
              {state.message}
            </p>
          )}
        </div>
      </form>
    </section>
  )
}
