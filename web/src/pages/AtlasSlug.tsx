import { useParams } from 'react-router'
import AtlasEntry from '@/pages/AtlasEntry'
import AtlasListing, { listingBySlug } from '@/pages/AtlasListing'

/**
 * `/atlas/:slug` addresses two different kinds of page, and the slug is what
 * says which: slugs are unique across the community atlas and the WebMCP
 * directory (scripts/lib/directory.ts refuses a listing that collides with an
 * entry), so a lookup in `directoryListings` is enough to decide.
 *
 * It is a dispatcher rather than two routes because the route table is read by
 * scripts/check-route-coverage.ts, which pairs each `<Route path>` in App.tsx
 * with a prerendered file — two literal routes for one URL shape would have to
 * be explained to it twice.
 */
export default function AtlasSlug() {
  const { slug = '' } = useParams()
  const listing = listingBySlug(slug)
  // Not `listing ? … : …` with the listing looked up inside AtlasListing: the
  // component still owns the "unknown slug" redirect, and passing `undefined`
  // through keeps that decision in one place for listings while AtlasEntry
  // keeps its own for entries.
  return listing ? <AtlasListing listing={listing} /> : <AtlasEntry />
}
