import { Link } from 'react-router'
import AtlasMark from './AtlasMark'
import { formatDate } from '@/components/BlogCard'
import { primaryDomain, statParts } from '@/lib/atlas'
import type { AtlasEntry } from '@/types/atlas'

export default function AtlasCard({ entry }: { entry: AtlasEntry }) {
  const domain = primaryDomain(entry)
  const shot = entry.screenshotUrls[0]

  return (
    <Link to={`/atlas/${entry.slug}`} className="atlas-card" data-component="AtlasCard">
      <div className="atlas-card__shot">
        {shot ? (
          // Screenshots are 1600px wide and the card is far narrower, so this
          // is always downscaled — lazy + async decode keeps a long gallery
          // from blocking first paint. Dimensions are not in the schema, so no
          // width/height here; the CSS aspect-ratio box reserves the space
          // instead and the layout does not shift when the image lands.
          <img src={shot} alt={`Screenshot of ${entry.name}`} loading="lazy" decoding="async" />
        ) : (
          // Entries are allowed to ship no screenshots. Fall back to the
          // domain set as type rather than leaving a grey hole.
          <div className="atlas-card__shot-fallback">
            <span>{domain}</span>
          </div>
        )}
      </div>

      <div className="atlas-card__body">
        <div className="atlas-card__head">
          <AtlasMark domain={domain} />
          <span className="atlas-card__name">{entry.name}</span>
          <span className="atlas-card__method">{entry.method}</span>
        </div>

        <p className="atlas-card__desc">{entry.description}</p>

        <div className="atlas-card__stats">
          {statParts(entry).map((part, i) => (
            <span key={part.label}>
              {i > 0 && <span className="atlas-card__dot"> · </span>}
              <strong>{part.value}</strong> {part.label}
            </span>
          ))}
        </div>

        <div className="atlas-card__foot">
          <span>{entry.author}</span>
          <time dateTime={entry.updated}>{formatDate(entry.updated)}</time>
        </div>
      </div>
    </Link>
  )
}
