import { useEffect, useState } from 'react'
import { useSearchParams } from 'react-router'
import Navigation from '@/components/Navigation'
import Footer from '@/components/Footer'
import Seo from '@/components/Seo'
import AtlasCard from '@/components/atlas/AtlasCard'
import AtlasSubmitCard from '@/components/atlas/AtlasSubmitCard'
import { filterEntries } from '@/lib/atlas'
import { atlasCategories, atlasEntries } from '@/generated/atlas-manifest'
// Shared with scripts/prerender.tsx (this page's meta description/title) — see
// the note on ATLAS_DESCRIPTION for why the constants live there.
import { ATLAS_DESCRIPTION, ATLAS_INDEX_TITLE } from '../../scripts/lib/site'

export default function AtlasIndex() {
  const [params, setParams] = useSearchParams()

  // The filters live in the URL so a filtered view is linkable, which means a
  // visitor can arrive at /atlas?category=docs — but scripts/prerender.tsx
  // renders exactly one static file for this route, unfiltered, and Netlify
  // serves it for every query string. src/main.tsx compares only the pathname
  // before hydrating, so it *will* hydrate that unfiltered markup against a
  // URL that asks for a filter. Reading the params during the first render
  // would therefore be a guaranteed hydration mismatch.
  //
  // So: first render always ignores them, and this effect (which never runs
  // during renderToString, and runs before paint on the client) switches them
  // on. The cost is that a deep-linked filter applies one commit late; the
  // alternative is a mismatch that blows away the whole tree.
  const [hydrated, setHydrated] = useState(false)
  useEffect(() => setHydrated(true), [])

  const category = hydrated ? (params.get('category') ?? '') : ''
  const query = hydrated ? (params.get('q') ?? '') : ''

  // Empty values are dropped rather than written as `?q=`, so the URL a
  // visitor copies is the shortest one that reproduces what they see.
  const update = (next: { category?: string; q?: string }, replace: boolean) => {
    const merged = { category, q: query, ...next }
    const out = new URLSearchParams()
    if (merged.category) out.set('category', merged.category)
    if (merged.q) out.set('q', merged.q)
    setParams(out, { replace })
  }

  const shown = filterEntries(atlasEntries, category, query)
  const filtered = Boolean(category || query)

  return (
    <>
      <Seo title={ATLAS_INDEX_TITLE} description={ATLAS_DESCRIPTION} />
      <Navigation />
      <main className="atlas-index" data-component="AtlasIndex">
        <div className="container container--wide">
          <div className="atlas-index__header">
            <div className="section-label">Atlas</div>
            <h1>Sightmaps of real sites</h1>
            <p className="section-desc">{ATLAS_DESCRIPTION}</p>
            <p className="atlas-index__machine">
              For agents:{' '}
              <a href="/atlas/index.json">
                <code>/atlas/index.json</code>
              </a>
            </p>
          </div>

          {atlasEntries.length > 0 && (
            <div className="atlas-filters" data-component="AtlasFilters">
              <label className="atlas-filters__search">
                <span className="sr-only">Search the atlas</span>
                <input
                  type="search"
                  placeholder="Search sites…"
                  value={query}
                  // Typing replaces rather than pushes: one history entry per
                  // keystroke would make the back button unusable. Picking a
                  // category is a discrete choice, so that one pushes.
                  onChange={(e) => update({ q: e.target.value }, true)}
                />
              </label>

              {atlasCategories.length > 0 && (
                <div className="atlas-filters__cats">
                  <button
                    type="button"
                    className={`atlas-chip${category === '' ? ' atlas-chip--on' : ''}`}
                    aria-pressed={category === ''}
                    onClick={() => update({ category: '' }, false)}
                  >
                    All
                  </button>
                  {atlasCategories.map((cat) => (
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
          {atlasEntries.length === 0 ? (
            <p className="atlas-index__empty">
              No entries yet.{' '}
              <a href="https://github.com/sightmap/atlas" target="_blank" rel="noreferrer">
                Contribute the first one
              </a>
              .
            </p>
          ) : shown.length === 0 ? (
            <p className="atlas-index__empty">
              No entries match.{' '}
              <button type="button" className="atlas-index__clear" onClick={() => update({ category: '', q: '' }, false)}>
                Clear filters
              </button>
            </p>
          ) : (
            <>
              <div className="atlas-cards">
                {shown.map((entry) => (
                  <AtlasCard key={entry.slug} entry={entry} />
                ))}
                {/* Last in the grid, filtered or not: someone who narrowed to a
                    category and found a gap is exactly who should see it. It is
                    not counted in the line below, which counts entries. */}
                <AtlasSubmitCard />
              </div>
              {/* Unfiltered, "1 of 1 entry" is noise — just say how many there
                  are. The x-of-y form only earns its place once a filter is
                  hiding something. */}
              <p className="atlas-index__count">
                {filtered
                  ? `${shown.length} of ${atlasEntries.length} shown`
                  : `${atlasEntries.length} ${atlasEntries.length === 1 ? 'entry' : 'entries'}`}
              </p>
            </>
          )}
        </div>
      </main>
      <Footer />
    </>
  )
}
