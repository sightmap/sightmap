import { useEffect, useState } from 'react'
import { useSearchParams } from 'react-router'
import Navigation from '@/components/Navigation'
import Footer from '@/components/Footer'
import Seo from '@/components/Seo'
import AtlasCard from '@/components/atlas/AtlasCard'
import DirectoryCard from '@/components/atlas/DirectoryCard'
import AtlasSubmitCard from '@/components/atlas/AtlasSubmitCard'
import AtlasSubmitForm from '@/components/atlas/AtlasSubmitForm'
import { filterDirectory, filterEntries } from '@/lib/atlas'
import {
  atlasCategories,
  atlasEntries,
  directoryCategories,
  directoryListings,
} from '@/generated/atlas-manifest'
// Shared with scripts/prerender.tsx (this page's meta description/title) — see
// the note on ATLAS_DESCRIPTION for why the constants live there.
import { ATLAS_DESCRIPTION, ATLAS_INDEX_TITLE } from '../../scripts/lib/site'

/**
 * The gallery holds two kinds of thing and the type row is how a visitor tells
 * them apart: a **listing** is a site whose live pages registered callable
 * WebMCP tools when Atlas scanned it (live or demo), and a **community map**
 * is a vendored sightmap corpus of a site that exposes none. Both are useful
 * to an agent; only the first is a claim about the site's own surface.
 */
const TYPES = [
  { id: '', label: 'All' },
  { id: 'live', label: 'Live' },
  { id: 'demo', label: 'Demo' },
  { id: 'community', label: 'Community maps' },
] as const

/** The three steps between a submitted URL and a published listing. */
const HOW = [
  {
    n: '01',
    title: 'Scan',
    body: 'A browser session enumerates the WebMCP tools each page registers, and records their input schemas. No tool is executed.',
  },
  {
    n: '02',
    title: 'Review',
    body: 'A review agent drafts the listing — description, category, tool classification, journeys worth trying — and a maintainer reviews and corrects it before it ships.',
  },
  {
    n: '03',
    title: 'Report',
    body: 'You get the scan report, a share card, and a Sightkick starter.',
  },
]

export default function AtlasIndex() {
  const [params, setParams] = useSearchParams()

  // One static file serves every query string for this route, so reading params
  // on the first render would be a guaranteed hydration mismatch. First render
  // ignores them; this effect switches them on one commit later. The submit
  // form's prefill rides the same rule.
  const [hydrated, setHydrated] = useState(false)
  useEffect(() => setHydrated(true), [])

  const category = hydrated ? (params.get('category') ?? '') : ''
  const query = hydrated ? (params.get('q') ?? '') : ''
  const rawType = hydrated ? (params.get('type') ?? '') : ''
  // An unknown ?type= is treated as no filter rather than as an empty gallery:
  // a typo in a shared link should show everything, not nothing.
  const type = TYPES.some((t) => t.id === rawType) ? rawType : ''
  const prefillUrl = hydrated ? (params.get('url') ?? '') : ''
  const rescan = hydrated ? params.get('rescan') === '1' : false
  const submittedId = hydrated ? (params.get('submitted') ?? '') : ''
  const errorCode = hydrated ? (params.get('error') ?? '') : ''

  // Empty values are dropped rather than written as `?q=`, so the URL a
  // visitor copies is the shortest one that reproduces what they see.
  const update = (next: { category?: string; q?: string; type?: string }, replace: boolean) => {
    const merged = { category, q: query, type, ...next }
    const out = new URLSearchParams()
    if (merged.type) out.set('type', merged.type)
    if (merged.category) out.set('category', merged.category)
    if (merged.q) out.set('q', merged.q)
    // The submit-form params are deliberately not carried: they describe a
    // one-time arrival (a rescan link, a no-JS redirect), not a view.
    setParams(out, { replace })
  }

  // Categories are the union of both vocabularies — an entry's `categories`
  // list and a listing's single `category` — so one chip row filters the whole
  // gallery instead of only half of it.
  const categories = [...new Set([...atlasCategories, ...directoryCategories])].sort()

  const shownEntries = type === '' || type === 'community' ? filterEntries(atlasEntries, category, query) : []
  const shownListings =
    type === 'community' ? [] : filterDirectory(directoryListings, category, query).filter((l) => !type || l.type === type)

  // One grid, sorted newest-first across both kinds: a listing scanned
  // yesterday should sit above a map vendored last month, and neither kind
  // gets a block of its own.
  const items = [
    ...shownListings.map((listing) => ({ kind: 'listing' as const, slug: listing.slug, updated: listing.updated, listing })),
    ...shownEntries.map((entry) => ({ kind: 'entry' as const, slug: entry.slug, updated: entry.updated, entry })),
  ].sort((a, b) => b.updated.localeCompare(a.updated) || a.slug.localeCompare(b.slug))

  const total = atlasEntries.length + directoryListings.length
  const filtered = Boolean(category || query || type)

  return (
    <>
      <Seo title={ATLAS_INDEX_TITLE} description={ATLAS_DESCRIPTION} />
      <Navigation />
      <main className="atlas-index" data-component="AtlasIndex">
        <div className="container container--wide">
          <div className="atlas-index__header">
            <div className="section-label">Atlas</div>
            <h1>Apps with WebMCP tools, and maps of the ones without</h1>
            <p className="section-desc">{ATLAS_DESCRIPTION}</p>
            <p className="atlas-index__machine">
              For agents:{' '}
              <a href="/atlas/directory.json">
                <code>/atlas/directory.json</code>
              </a>{' '}
              ·{' '}
              <a href="/atlas/stats.json">
                <code>/atlas/stats.json</code>
              </a>{' '}
              ·{' '}
              <a href="/atlas/index.json">
                <code>/atlas/index.json</code>
              </a>
            </p>
          </div>

          {total > 0 && (
            <div className="atlas-filters" data-component="AtlasFilters">
              <label className="atlas-filters__search">
                <span className="sr-only">Search the atlas</span>
                <input
                  type="search"
                  placeholder="Search sites and tools…"
                  value={query}
                  // Typing replaces rather than pushes: one history entry per
                  // keystroke would make the back button unusable. Picking a
                  // type or a category is a discrete choice, so those push.
                  onChange={(e) => update({ q: e.target.value }, true)}
                />
              </label>

              {/* Ahead of the categories, because it is the coarser cut: what
                  kind of thing this is, before what it is about. */}
              <div className="atlas-filters__types" data-component="AtlasTypeFilter">
                {TYPES.map((t) => (
                  <button
                    key={t.id || 'all'}
                    type="button"
                    className={`atlas-chip${type === t.id ? ' atlas-chip--on' : ''}`}
                    aria-pressed={type === t.id}
                    onClick={() => update({ type: t.id }, false)}
                  >
                    {t.label}
                  </button>
                ))}
              </div>

              {categories.length > 0 && (
                <div className="atlas-filters__cats">
                  <button
                    type="button"
                    className={`atlas-chip${category === '' ? ' atlas-chip--on' : ''}`}
                    aria-pressed={category === ''}
                    onClick={() => update({ category: '' }, false)}
                  >
                    All
                  </button>
                  {categories.map((cat) => (
                    <button
                      key={cat}
                      type="button"
                      className={`atlas-chip${category === cat ? ' atlas-chip--on' : ''}`}
                      aria-pressed={category === cat}
                      // Clicking the active category clears it, so the chip row
                      // is a toggle and there is no dead click.
                      onClick={() => update({ category: category === cat ? '' : cat }, false)}
                    >
                      {cat}
                    </button>
                  ))}
                </div>
              )}
            </div>
          )}

          {/*
            Three empty states, not one: nothing published yet, nothing matching
            the filters, and the normal grid. Collapsing the first two would
            tell a visitor the atlas is empty when it is only their query that
            is.
          */}
          {total === 0 ? (
            <p className="atlas-index__empty">
              Nothing listed yet.{' '}
              <a href="#submit">Submit the first site</a>.
            </p>
          ) : items.length === 0 ? (
            <p className="atlas-index__empty">
              Nothing matches.{' '}
              <button
                type="button"
                className="atlas-index__clear"
                onClick={() => update({ category: '', q: '', type: '' }, false)}
              >
                Clear filters
              </button>
            </p>
          ) : (
            <>
              <div className="atlas-cards">
                {items.map((item) =>
                  item.kind === 'listing' ? (
                    <DirectoryCard key={`l:${item.slug}`} listing={item.listing} />
                  ) : (
                    <AtlasCard key={`e:${item.slug}`} entry={item.entry} />
                  )
                )}
                {/* Last in the grid, filtered or not: someone who narrowed to a
                    category and found a gap is exactly who should see it. It is
                    not counted in the line below, which counts listings and
                    entries. */}
                <AtlasSubmitCard />
              </div>
              {/* Unfiltered, "1 of 1" is noise — just say how many there are.
                  The x-of-y form only earns its place once a filter is hiding
                  something. */}
              <p className="atlas-index__count">
                {filtered
                  ? `${items.length} of ${total} shown`
                  : `${shownListings.length} ${shownListings.length === 1 ? 'listing' : 'listings'} · ${shownEntries.length} community ${shownEntries.length === 1 ? 'map' : 'maps'}`}
              </p>
            </>
          )}

          <section className="atlas-how" aria-labelledby="atlas-how-h" data-component="AtlasHow">
            <h2 id="atlas-how-h" className="atlas-listing__h2">
              How listing works
            </h2>
            <ol className="atlas-how__steps">
              {HOW.map((step) => (
                <li key={step.n} className="atlas-how__step">
                  <span className="atlas-how__n">{step.n}</span>
                  <span className="atlas-how__title">{step.title}</span>
                  <p className="atlas-how__body">{step.body}</p>
                </li>
              ))}
            </ol>
          </section>

          <AtlasSubmitForm initialUrl={prefillUrl} rescan={rescan} submittedId={submittedId || undefined} errorCode={errorCode || undefined} />

          <p className="atlas-index__note">
            Tool classification is Atlas&rsquo;s own and is corrected by a maintainer on review.
            A listing records the tools a scan detected on the date shown, and says nothing about
            the site beyond that.
          </p>
        </div>
      </main>
      <Footer />
    </>
  )
}
