import { Link } from 'react-router'
import AtlasMark from './AtlasMark'
import { formatScanned } from '@/lib/directory'
import type { DirectoryListing } from '@/types/directory'

/**
 * One WebMCP listing in the gallery grid, the directory's counterpart to
 * `AtlasCard`. It shares `.atlas-card`'s shell so the mixed grid keeps a
 * single card shape, and diverges where a listing genuinely differs from a
 * mapped entry: there is no screenshot to show, and the fact worth the top of
 * the card is the tool count and how those tools were classified by Atlas.
 *
 * Everything on the card is observable: a count, a surface, a date. No grade,
 * no stars — the whole point of the Atlas is that a listing is evidence a scan
 * found these tools on the date shown.
 */
export default function DirectoryCard({ listing }: { listing: DirectoryListing }) {
  const { counts } = listing
  const journeyPassed = listing.journey?.outcome === 'passed'

  return (
    <Link to={`/atlas/${listing.slug}`} className="atlas-card atlas-card--listing" data-component="DirectoryCard">
      {/* In place of the screenshot band: the one number a visitor is here
          for, with the Atlas classification broken out beneath it. */}
      <div className="atlas-card__strip">
        <span className="atlas-card__toolcount">
          <strong>{counts.tools}</strong> {counts.tools === 1 ? 'tool' : 'tools'}
        </span>
        <span className="atlas-card__kinds">
          {counts.read} read · {counts.action} action · {counts.sensitive} sensitive
        </span>
      </div>

      <div className="atlas-card__body">
        <div className="atlas-card__head">
          <AtlasMark domain={listing.host} />
          <span className="atlas-card__name">{listing.name}</span>
          <span className={`atlas-type atlas-type--${listing.type}`}>{listing.type}</span>
        </div>

        <p className="atlas-card__desc">{listing.description}</p>

        <div className="atlas-card__stats">
          <strong>{listing.surface}</strong> surface
          <span className="atlas-card__dot"> · </span>
          {counts.pages} {counts.pages === 1 ? 'page' : 'pages'} checked
        </div>

        {(listing.built_with_sightkick || journeyPassed) && (
          <div className="atlas-card__badges">
            {listing.built_with_sightkick && <span className="atlas-badge">Built with Sightkick</span>}
            {journeyPassed && <span className="atlas-badge atlas-badge--journey">Journey passed</span>}
          </div>
        )}

        <div className="atlas-card__foot">
          <span>{listing.category}</span>
          {/* The scan date, not the listing date: what a visitor needs to know
              is how stale the tool list is. */}
          <time dateTime={listing.scannedAt}>scanned {formatScanned(listing.scannedAt)}</time>
        </div>
      </div>
    </Link>
  )
}
