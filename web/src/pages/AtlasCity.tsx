import { useMemo } from 'react'
import { Link } from 'react-router'
import Navigation from '@/components/Navigation'
import Footer from '@/components/Footer'
import Seo from '@/components/Seo'
import CityView from '@/components/city/CityView'
import { cityCaption, cityStats, type CityListing } from '@/components/city/document'
import { city, directoryListings } from '@/generated/atlas-manifest'
// Shared with scripts/prerender.tsx, so a client-side arrival and a fresh load
// of this route carry the same <title>.
import { ATLAS_CITY_DESCRIPTION, ATLAS_CITY_TITLE } from '../../scripts/lib/site'

/**
 * The city. Every admitted listing is a building on a lot it keeps, and the
 * rest of the plan is what the directory has not grown into yet — which is the
 * honest picture of a gallery with eight entries and room for three hundred.
 */
export default function AtlasCity() {
  const listings = useMemo<CityListing[]>(
    () => directoryListings.map((l) => ({ slug: l.slug, name: l.name, blueprint: l.blueprint })),
    []
  )
  const stats = useMemo(() => cityStats(city), [])

  return (
    <>
      <Seo title={ATLAS_CITY_TITLE} description={ATLAS_CITY_DESCRIPTION} />
      <Navigation />
      <main className="atlas-city" data-component="AtlasCity">
        <div className="container container--wide">
          <div className="atlas-city__header">
            <div className="section-label">Atlas</div>
            <h1>The city</h1>
            <p className="section-desc">
              Every listed site is a building, derived from its scan; unclaimed lots are filler
              until a listing takes them.
            </p>
            <p className="atlas-city__stats">{cityCaption(stats)}</p>
          </div>

          <CityView city={city} listings={listings} />

          <p className="atlas-city__note">
            A building&rsquo;s floors are the pages a scan reached and its height is what its
            address allows, so the skyline is the directory&rsquo;s shape and not a ranking.
            Nothing here is an endorsement. Click a building to look inside it, then again to open
            its listing — or start from <Link to="/atlas">the gallery</Link>.
          </p>
        </div>
      </main>
      <Footer />
    </>
  )
}
