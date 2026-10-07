// Browser-safe atlas helpers. Deliberately free of node: imports and of any
// dependency on scripts/lib/atlas.ts (which reads the filesystem) so this can
// be pulled into the bundle and can also run under scripts/prerender.tsx.
import type { AtlasEntry } from '@/types/atlas'
import type { DirectoryListing } from '@/types/directory'

/**
 * The domain to show as the entry's identity. `domains` is the schema's answer
 * and comes first; `site_url`'s host is the fallback for an entry that lists
 * none, and the raw `site_url` is the last resort so this never returns ''.
 */
export function primaryDomain(entry: Pick<AtlasEntry, 'domains' | 'site_url'>): string {
  if (entry.domains.length > 0) return entry.domains[0]
  try {
    return new URL(entry.site_url).hostname
  } catch {
    return entry.site_url
  }
}

// Site marks are drawn locally from the domain, never fetched from the site
// itself or from a favicon service. Two reasons, and the first is a ground
// rule: the atlas pages make no network call at build *or* run time, and an
// <img> pointed at a community-submitted domain is a run-time call made from
// the visitor's browser. It would also hand every listed third party (and any
// favicon proxy in between) the IP of everyone who loads /atlas, on a site
// that gates its own analytics behind a consent banner.
const MARK_COLORS = ['var(--accent)', 'var(--blue)', 'var(--purple)', 'var(--green)', 'var(--yellow)']

/** Stable per-domain accent, so an entry keeps the same mark across builds. */
export function markColor(domain: string): string {
  let h = 0
  for (let i = 0; i < domain.length; i++) h = (h * 31 + domain.charCodeAt(i)) >>> 0
  return MARK_COLORS[h % MARK_COLORS.length]
}

/** The letter drawn in the mark: the domain's first alphanumeric, uppercased. */
export function markInitial(domain: string): string {
  return (domain.match(/[a-z0-9]/i)?.[0] ?? '?').toUpperCase()
}

export interface StatPart {
  label: string
  value: number
}

/**
 * The `N views · M components · K requests` line. Returns parts rather than a
 * string so the card can style the numbers, and singularizes so a one-view
 * entry doesn't read "1 views".
 */
export function statParts(entry: Pick<AtlasEntry, 'stats'>): StatPart[] {
  const { views, components, requests } = entry.stats
  return [
    { label: views === 1 ? 'view' : 'views', value: views },
    { label: components === 1 ? 'component' : 'components', value: components },
    { label: requests === 1 ? 'request' : 'requests', value: requests },
  ]
}

/** `FIG. 01`-style caption numbering for the screenshot gallery. */
export function figLabel(index: number): string {
  return `FIG. ${String(index + 1).padStart(2, '0')}`
}

/**
 * Case-insensitive substring match across the fields a visitor would plausibly
 * type: slug, name, domains, description, categories, author.
 *
 * The slug is in here because the CLI's `sightmap atlas find` searches slug,
 * name, domains, categories and description against this same index.json, and
 * the slug is what someone pastes back after reading it off a card. Typing one
 * query into the grid and into the CLI should return the same entries.
 */
export function matchesQuery(entry: AtlasEntry, query: string): boolean {
  const q = query.trim().toLowerCase()
  if (!q) return true
  const haystack = [
    entry.slug,
    entry.name,
    entry.description,
    entry.author,
    ...entry.domains,
    ...entry.categories,
  ]
    .join(' ')
    .toLowerCase()
  return haystack.includes(q)
}

export function filterEntries(entries: AtlasEntry[], category: string, query: string): AtlasEntry[] {
  return entries.filter(
    (e) => (!category || e.categories.includes(category)) && matchesQuery(e, query)
  )
}

/**
 * The directory half of the gallery's search. Same contract as
 * `matchesQuery` above — case-insensitive substring across the fields a
 * visitor would plausibly type — but over a listing, whose identity is the
 * host and whose interesting nouns are the tool names.
 *
 * Tool names are in the haystack because "checkout" or "search_flights" is
 * exactly what someone hunting for an agent-callable site types, and it is the
 * one thing a listing has that an atlas entry does not.
 */
export function matchesListingQuery(listing: DirectoryListing, query: string): boolean {
  const q = query.trim().toLowerCase()
  if (!q) return true
  const haystack = [
    listing.slug,
    listing.name,
    listing.host,
    listing.description,
    listing.category,
    listing.type,
    ...listing.labels,
    ...listing.collections,
    ...listing.tools.map((t) => t.name),
  ]
    .join(' ')
    .toLowerCase()
  return haystack.includes(q)
}

export function filterDirectory<T extends DirectoryListing>(listings: T[], category: string, query: string): T[] {
  return listings.filter((l) => (!category || l.category === category) && matchesListingQuery(l, query))
}
