// The wire contract between the Atlas submit form and the Netlify function
// that receives it. Kept in src/ (not netlify/) because the browser bundle is
// the side that has to spell the endpoint and the field names correctly, and
// a shared module is the only way the two cannot drift.
//
// Browser-safe: no node: imports, no side effects.

/**
 * POST target. Also the `action` of the no-JS <form>, so it must be a path.
 *
 * Netlify's request chain runs serverless functions *before* redirects and
 * rewrites (edge functions → cache → functions → redirects → static files →
 * 404 handler), so this function's `config.path` wins over both the
 * `/api/atlas/:slug` rewrite and the `/*` → `/404.html` catch-all in
 * netlify.toml. Two things do run earlier and are handled explicitly:
 * the `negotiate` edge function, which answers `/api/*` itself and passes
 * this path through (`FUNCTION_PATHS` in netlify/lib/handler.ts).
 *
 * Setting a custom `path` normally *removes* the default function URL
 * ("the function is only available at that path"), so the function declares
 * both paths and SUBMIT_ENDPOINT_DIRECT below stays live as an alias.
 */
export const SUBMIT_ENDPOINT = '/api/atlas/submit'

/** The default function URL, declared alongside the pretty one. Same handler. */
export const SUBMIT_ENDPOINT_DIRECT = '/.netlify/functions/atlas-submit'

/** Host lookup for a scanned site: `/api/atlas/lookup/<host>` → static JSON. */
export const LOOKUP_ENDPOINT_PREFIX = '/api/atlas/lookup/'

/** Where a no-JS form post lands on success: `/atlas?submitted=<id>`. */
export const SUBMITTED_QUERY_PARAM = 'submitted'

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
  error?: { code: string; message: string; hint?: string; status?: number }
}

/** Field names, so the form markup and the handler cannot drift apart. */
export const SUBMIT_FIELDS = [
  'url',
  'email',
  'owner',
  'sightkick',
  'intent',
  'nominate',
  'rescan',
  'website',
] as const

export const MAX_EMAIL_LENGTH = 254
export const MAX_INTENT_LENGTH = 500

/** How many submissions one client may make per rolling window. */
export const RATE_LIMIT_MAX = 5
export const RATE_LIMIT_WINDOW_MS = 24 * 60 * 60 * 1000

/** Lifecycle of a submission, as far as the public API exposes it. */
export type SubmitState = 'new' | 'scanning' | 'in-review' | 'listed' | 'rejected'

/** Which runner picked the submission up. `queue` means "a maintainer will". */
export type RunnerKind = 'netlify' | 'github' | 'queue'

/** 202 response to a JSON POST. */
export interface SubmitAccepted {
  ok: true
  id: string
  state: 'received'
  message: string
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

export type SubmitResult = SubmitAccepted | SubmitError
