// The wire contract between the Atlas submit form and the Netlify function
// that receives it. Kept in src/ (not netlify/) because the browser bundle is
// the side that has to spell the endpoint and the field names correctly, and
// a shared module is the only way the two cannot drift.
//
// Browser-safe: no node: imports, no side effects.

/**
 * POST target, and the action of the no-JS <form>, so it must be a path.
 * Netlify runs functions before redirects, so this wins over netlify.toml's
 * rules — see netlify/functions/atlas-submit.mts for the full chain.
 */
export const SUBMIT_ENDPOINT = '/api/atlas/submit'

export interface SubmitRequest {
  url: string
  email: string
  /** The submitter maintains the site. */
  owner?: boolean
  /** The site was built with Sightkick. */
  sightkick?: boolean
  /** What the submitter wants an agent to be able to do. Stored, not acted on. */
  intent?: string
  /** Someone else's site, submitted by a third party. */
  nominate?: boolean
  /** Re-scan of a site that is already listed. */
  rescan?: boolean
  /**
   * The token in the submitter's `webmcp.txt` claim line. Proves control of
   * the host, which is what an unlisted card at /try/<host> stands on. It is
   * compared during the request and never stored.
   */
  claim?: string
  /** Honeypot. A real submitter never fills this; a bot fills everything. */
  website?: string
}

/**
 * Loose response shape for callers that just want `ok` and a message. The
 * discriminated pair below (`SubmitAccepted` | `SubmitError`) is the precise
 * version; both describe the same bytes.
 */
export interface SubmitResponse {
  ok: boolean
  id?: string
  state?: string
  message?: string
  card?: string
  error?: { code: string; message: string; hint?: string; status?: number }
}

export const MAX_EMAIL_LENGTH = 254
export const MAX_INTENT_LENGTH = 500

/**
 * 16 random bytes as lowercase hex — `openssl rand -hex 16`. The `<input>`
 * pattern, the endpoint's validation, and the OpenAPI schema all read it from
 * here so a client cannot be told one rule and checked against another.
 */
export const CLAIM_TOKEN_PATTERN = '[0-9a-f]{32}'
export const CLAIM_TOKEN_LENGTH = 32

/** How many submissions one client may make per rolling window. */
export const RATE_LIMIT_MAX = 5
export const RATE_LIMIT_WINDOW_MS = 24 * 60 * 60 * 1000

/**
 * Lifecycle of a submission, as far as the public API exposes it. `queued`
 * means it was accepted and stored but no runner was triggered for it — the
 * global daily ceiling was reached, so a maintainer runs the intake by hand.
 */
export type SubmitState = 'new' | 'queued' | 'scanning' | 'in-review' | 'listed' | 'rejected'

/** Which runner picked the submission up. `queue` means "a maintainer will". */
export type RunnerKind = 'netlify' | 'github' | 'queue'

/** 202 response to a JSON POST. */
export interface SubmitAccepted {
  ok: true
  id: string
  state: 'received'
  message: string
  /**
   * `https://sightmap.org/try/<host>`, present only when the submission
   * carried a claim that verified. An unlisted page, not an Atlas listing.
   */
  card?: string
}

/** 200 response to `GET /api/atlas/submit?id=…`. Never echoes url or email. */
export interface SubmitStatus {
  ok: true
  id: string
  state: SubmitState
  receivedAt: string
  host: string
  runner: { kind: RunnerKind; state?: string }
}

/** Any 4xx/5xx from the endpoint. `hint` and `status` mirror the /api/* errors. */
export interface SubmitError {
  ok: false
  error: {
    code: string
    message: string
    hint: string
    status: number
  }
}
