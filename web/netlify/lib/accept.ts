// RFC 9110 Accept parsing for content negotiation.
//
// Ranking: higher q wins; ties break by specificity (type/subtype >
// type/* > */*). q=0 is an explicit refusal. A missing Accept header is
// "no constraint" and the caller should serve its default — it is not
// the same as an empty list.
//
// Follows the algorithm at acceptmarkdown.com/guides/accept-parsing.

export interface AcceptEntry {
  type: string
  subtype: string
  q: number
  specificity: number
}

const SPECIFICITY_EXACT = 3
const SPECIFICITY_SUBTYPE_WILD = 2
const SPECIFICITY_TYPE_WILD = 1

function parseQ(raw: string | undefined): number {
  if (raw === undefined || raw === '') return 1
  const n = Number(raw)
  if (!Number.isFinite(n) || n < 0) return 0
  if (n > 1) return 1
  return n
}

function specificity(type: string, subtype: string): number {
  if (type === '*' && subtype === '*') return SPECIFICITY_TYPE_WILD
  if (subtype === '*') return SPECIFICITY_SUBTYPE_WILD
  return SPECIFICITY_EXACT
}

/** Split an Accept header into typed entries. Invalid pieces are dropped. */
export function parseAccept(header: string | null | undefined): AcceptEntry[] {
  if (header === null || header === undefined) return []
  const trimmed = header.trim()
  if (trimmed === '') return []

  const entries: AcceptEntry[] = []
  for (const part of trimmed.split(',')) {
    const params = part.split(';').map((s) => s.trim()).filter(Boolean)
    const media = params.shift()
    if (!media) continue
    const slash = media.indexOf('/')
    if (slash <= 0 || slash === media.length - 1) continue
    const type = media.slice(0, slash).toLowerCase()
    const subtype = media.slice(slash + 1).toLowerCase()
    if (!type || !subtype) continue

    let q = 1
    for (const param of params) {
      const eq = param.indexOf('=')
      if (eq === -1) continue
      const name = param.slice(0, eq).trim().toLowerCase()
      if (name !== 'q') continue
      q = parseQ(param.slice(eq + 1).trim())
    }

    entries.push({ type, subtype, q, specificity: specificity(type, subtype) })
  }
  return entries
}

function matches(offered: string, entry: AcceptEntry): boolean {
  const slash = offered.indexOf('/')
  if (slash <= 0) return false
  const type = offered.slice(0, slash).toLowerCase()
  const subtype = offered.slice(slash + 1).toLowerCase()
  if (entry.type === '*' && entry.subtype === '*') return true
  if (entry.type === type && entry.subtype === '*') return true
  return entry.type === type && entry.subtype === subtype
}

/**
 * Pick the offered type the client prefers, or null when nothing matches
 * (the caller should return 406). `defaultType` is used when Accept is
 * missing or every matching entry is a wildcard (star/star or type/star) — i.e.
 * the client said "anything is fine".
 *
 * `defaultType` must be in `offered`.
 */
export function negotiate(
  header: string | null | undefined,
  offered: readonly string[],
  defaultType: string
): string | null {
  if (!offered.includes(defaultType)) {
    throw new Error(`negotiate: defaultType "${defaultType}" is not in offered`)
  }
  // Missing header = no constraint. Empty header is treated the same:
  // some clients send `Accept:` with nothing after it.
  if (header === null || header === undefined || header.trim() === '') {
    return defaultType
  }

  const entries = parseAccept(header)
  if (entries.length === 0) return defaultType

  let bestType: string | null = null
  let bestQ = 0
  let bestSpec = -1

  for (const type of offered) {
    let matchQ = 0
    let matchSpec = -1
    for (const entry of entries) {
      if (!matches(type, entry)) continue
      if (entry.q === 0) {
        // Explicit refusal of this class. A more specific positive match
        // (already recorded) still wins; a q=0 exact match knocks this
        // offered type out if it is the best match we have.
        if (entry.specificity > matchSpec) {
          matchQ = 0
          matchSpec = entry.specificity
        }
        continue
      }
      if (entry.specificity > matchSpec || (entry.specificity === matchSpec && entry.q > matchQ)) {
        matchQ = entry.q
        matchSpec = entry.specificity
      }
    }
    if (matchQ > bestQ || (matchQ === bestQ && matchQ > 0 && matchSpec > bestSpec)) {
      bestQ = matchQ
      bestSpec = matchSpec
      bestType = type
    }
  }

  if (bestQ === 0 || bestType === null) return null

  // Wildcards only (`*/*` or `text/*`) mean "anything is fine" — serve
  // the default rather than the first offered type that happened to match.
  if (bestSpec < SPECIFICITY_EXACT) return defaultType
  return bestType
}

/** Merge Vary tokens, canonicalizing Accept / Accept-Encoding. */
export function mergeVary(existing: string | null | undefined, extras: readonly string[]): string {
  const seen = new Map<string, string>()
  const add = (raw: string) => {
    const token = raw.trim()
    if (!token) return
    const key = token.toLowerCase()
    if (key === 'accept') seen.set(key, 'Accept')
    else if (key === 'accept-encoding') seen.set(key, 'Accept-Encoding')
    else if (!seen.has(key)) seen.set(key, token)
  }
  if (existing) {
    for (const part of existing.split(',')) add(part)
  }
  for (const extra of extras) add(extra)
  return [...seen.values()].join(', ')
}

/** Paths that are already a concrete representation — do not negotiate. */
export function isPassthroughPath(pathname: string): boolean {
  if (pathname.startsWith('/assets/')) return true
  // A trailing file extension means the client named a representation
  // (openapi.json, airbnb.md, llms.txt). Negotiating those would loop
  // when this function fetches a twin through the same origin.
  const last = pathname.split('/').pop() ?? ''
  return last.includes('.')
}

/**
 * Where the markdown twin of an HTML page lives in dist/. `/` → `/index.md`;
 * every other route drops a trailing slash and appends `.md`.
 */
export function markdownTwinPath(pathname: string): string {
  const cleaned = pathname.replace(/\/+$/, '') || '/'
  if (cleaned === '/') return '/index.md'
  return `${cleaned}.md`
}

export function normalizePathname(pathname: string): string {
  if (pathname === '/') return '/'
  return pathname.replace(/\/+$/, '') || '/'
}
