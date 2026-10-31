// What the city looks like without WebGL: the legend. A crawler, a browser
// with JavaScript off and a machine that cannot make a canvas all get the same
// thing a visitor is looking for anyway — which listing is where, and a link
// to each one. Nothing on this path imports three.js.
import { Link } from 'react-router'
import type { CityLegendEntry } from './document'

export interface CityPlaceholderProps {
  entries: CityLegendEntry[]
  /** Hidden once the scene has drawn its first frame, but still in the page. */
  hidden?: boolean
}

export default function CityPlaceholder({ entries, hidden = false }: CityPlaceholderProps) {
  return (
    <div className="atlas-city__legend" data-component="CityPlaceholder" hidden={hidden}>
      <h2 className="atlas-city__legend-h">The legend</h2>
      {entries.length === 0 ? (
        <p className="atlas-city__legend-empty">No listing has taken a lot yet.</p>
      ) : (
        <ol className="atlas-city__legend-list">
          {entries.map((entry) => (
            <li key={entry.slug} className="atlas-city__legend-item">
              <Link to={`/atlas/${entry.slug}`}>{entry.name}</Link>
              <span className="atlas-city__legend-meta">
                {`${entry.district} · lot ${entry.lot}${entry.peak ? ' · the peak' : ''}`}
              </span>
            </li>
          ))}
        </ol>
      )}
    </div>
  )
}
